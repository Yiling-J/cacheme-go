package cacheme_test

import (
	"testing"
	"time"

	cacheme "github.com/Yiling-J/cacheme-go"
	"github.com/stretchr/testify/require"
)

func TestSchemaToStore(t *testing.T) {
	// duplicate key pattern
	stores := []*cacheme.StoreSchema{
		{
			Name:    "Simple",
			Key:     "simple:{{.ID}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Simple2",
			Key:     "simple:{{.ID2}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
	}

	err := cacheme.SchemaToStore("", stores, false)
	require.NotNil(t, err)

	// duplicate name
	stores = []*cacheme.StoreSchema{
		{
			Name:    "Simple",
			Key:     "simple:{{.ID}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Simple",
			Key:     "simple2:{{.ID2}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
	}
	err = cacheme.SchemaToStore("", stores, false)
	require.NotNil(t, err)

	// ok
	stores = []*cacheme.StoreSchema{
		{
			Name:    "Simple",
			Key:     "simple:{{.ID}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
		{
			Name:    "Simple2",
			Key:     "simple2:{{.ID2}}",
			To:      "string",
			Version: 1,
			TTL:     5 * time.Minute,
		},
	}
	err = cacheme.SchemaToStore("", stores, false)
	require.Nil(t, err)

}
