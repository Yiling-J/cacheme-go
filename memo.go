// https://github.com/kristoff-it/redis-memolock
// https://redis.com/blog/caches-promises-locks/
package cacheme

import (
	"context"
	"errors"
	"time"

	"github.com/go-redis/redis/v8"
	uuid "github.com/satori/go.uuid"
)

// FetchFunc is the function that the caller should provide to compute the value if not present in Redis already.
// time.Duration defines for how long the value should be cached in Redis.
type FetchFunc = func() (string, time.Duration, error)

// ExternalFetchFunc has the same purpose as FetchFunc but works on the assumption that the value will be set in Redis and notificed on Pub/Sub by an external program
type ExternalFetchFunc = func() error

// LockRenewFunc is the function that RenewableFetchFunc will get as input and that must be called to extend a locks' life
type LockRenewFunc = func(time.Duration) error

// RenewableFetchFunc has the same purpose as FetchFunc but, when called, it is offered a function that allows to extend the lock
type RenewableFetchFunc = func(LockRenewFunc) (string, time.Duration, error)

// ErrTimeOut happens when the given timeout expires
var ErrTimeOut = errors.New("operation timed out")

// ErrLockRenew happens when trying to renew a lock that expired already
var ErrLockRenew = errors.New("unable to renew the lock")

// ErrClosing happens when calling Close(), all pending requests will be failed with this error
var ErrClosing = errors.New("operation canceled by close()")

type subRequest struct {
	name    string
	isUnsub bool
	resCh   chan []byte
}

// RedisMemoLock implements the "promise" mechanism
type RedisMemoLock struct {
	client        RedisClient
	resourceTag   string
	prefix        string
	lockTimeout   time.Duration
	subCh         chan subRequest
	notifCh       <-chan *redis.Message
	subscriptions map[string][]chan []byte
}

func (r *RedisMemoLock) dispatch() {
	for {
		select {
		// We have a new sub/unsub request
		case sub, ok := <-r.subCh:
			if !ok {
				// We are closing, close all pending channels
				for _, list := range r.subscriptions {
					for _, ch := range list {
						close(ch)
					}
				}
				return
			}

			switch sub.isUnsub {
			case false:
				list := r.subscriptions[sub.name]
				r.subscriptions[sub.name] = append(list, sub.resCh)
			case true:
				// TODO: there are better strategies...
				if list, ok := r.subscriptions[sub.name]; ok {
					newList := list[:0]
					for _, x := range list {
						if sub.resCh != x {
							newList = append(newList, x)
						}
					}
					for i := len(newList); i < len(list); i++ {
						_ = append(newList, nil)
					}
				}
			}
		// We have a new notification from Redis Pub/Sub
		case msg, ok := <-r.notifCh:
			if !ok {
				return
			}
			if listeners, ok := r.subscriptions[msg.Channel]; ok {
				for _, ch := range listeners {
					if ch != nil {
						ch <- []byte(msg.Payload)
						close(ch)
					}
				}
				r.subscriptions[msg.Channel] = nil
			}
		}
	}
}

// NewRedisMemoLock Creates a new RedisMemoLock instance
func NewRedisMemoLock(ctx context.Context, prefix string, client RedisClient, resourceTag string, lockTimeout time.Duration) (*RedisMemoLock, error) {
	pattern := resourceTag + "/notif:*"

	pubsub := client.PSubscribe(ctx, pattern)
	_, err := pubsub.Receive(ctx)
	if err != nil {
		return nil, err
	}

	result := RedisMemoLock{
		client:        client,
		resourceTag:   resourceTag,
		prefix:        prefix,
		lockTimeout:   lockTimeout,
		subCh:         make(chan subRequest),
		notifCh:       pubsub.Channel(),
		subscriptions: make(map[string][]chan []byte),
	}

	// Start the dispatch loop
	go result.dispatch()

	return &result, nil
}

// Close stops listening to Pub/Sub and resolves all pending subscriptions with ErrClosing.
func (r *RedisMemoLock) Close() {
	close(r.subCh)
}

func (r *RedisMemoLock) GetCached(ctx context.Context, key string) ([]byte, error) {
	resourceID := r.prefix + ":" + key

	return r.client.Get(ctx, resourceID).Bytes()
}

func (r *RedisMemoLock) DeleteCache(ctx context.Context, key string) error {
	resourceID := r.prefix + ":" + key

	return r.client.Del(ctx, resourceID).Err()
}

func (r *RedisMemoLock) GetCachedP(ctx context.Context, pipe redis.Pipeliner, key string) *redis.StringCmd {
	resourceID := r.prefix + ":" + key

	return pipe.Get(ctx, resourceID)
}

func (r *RedisMemoLock) Lock(ctx context.Context, key string) (bool, error) {
	reqUUID := uuid.NewV4().String()
	lockID := r.resourceTag + "/lock:" + key
	return r.client.SetNX(ctx, lockID, reqUUID, r.lockTimeout).Result()
}

func (r *RedisMemoLock) SetCache(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	resourceID := r.prefix + ":" + key
	notifID := r.resourceTag + "/notif:" + key
	lockID := r.resourceTag + "/lock:" + key

	pipe := r.client.Pipeline()
	pipe.Set(ctx, resourceID, value, ttl)
	pipe.Publish(ctx, notifID, value)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return err
	}
	_ = r.client.Del(ctx, lockID).Err()
	return nil
}

func (r *RedisMemoLock) AddGroup(ctx context.Context, group string, key string) error {
	resourceID := r.prefix + ":" + key
	return r.client.SAdd(ctx, group, resourceID).Err()
}

func (r *RedisMemoLock) Wait(ctx context.Context, key string) ([]byte, error) {
	resourceID := r.prefix + ":" + key
	notifID := r.resourceTag + "/notif:" + key

	subReq := subRequest{name: notifID, isUnsub: false, resCh: make(chan []byte, 1)}

	r.subCh <- subReq
	unsubRequest := subRequest{name: notifID, isUnsub: true, resCh: subReq.resCh}

	res, err := r.client.Get(ctx, resourceID).Bytes()
	if err != redis.Nil { // key is not missing
		if err != nil { // real error happened?
			r.subCh <- unsubRequest
			return nil, err
		}
		r.subCh <- unsubRequest
		return res, nil
	}

	select {
	case <-time.After(5 * time.Second):
		r.subCh <- unsubRequest
		return nil, ErrTimeOut
	// The request can fail if .Close() was called
	case res, ok := <-subReq.resCh:
		if !ok {
			return nil, ErrClosing
		}
		return res, nil
	}
}
