// Package redistx covers outer-handle with go-redis, whose transactions are
// MULTI blocks.
package redistx

import (
	"context"

	redis8 "github.com/go-redis/redis/v8"
	"github.com/redis/go-redis/v9"
)

func Pipelined(ctx context.Context, rdb *redis.Client) error {
	_, err := rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, "a", 1, 0)
		rdb.Set(ctx, "b", 2, 0) // want `^rdb bypasses the transaction opened with TxPipelined; use pipe instead \[outer-handle\]$`
		return nil
	})
	return err
}

// Inside Watch, the watched connection is tx. Its own pipeline is not read:
// the optimistic pattern reads and watches keys on tx inside it, before
// MULTI, on purpose. A write there is missed.
func Watched(ctx context.Context, rdb *redis.Client) error {
	return rdb.Watch(ctx, func(tx *redis.Tx) error {
		rdb.Set(ctx, "a", 1, 0) // want `^rdb bypasses the transaction opened with Watch; use tx instead \[outer-handle\]$`
		_, err := tx.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
			tx.Set(ctx, "b", 2, 0)
			return nil
		})
		return err
	}, "a")
}

func Universal(ctx context.Context, rdb redis.UniversalClient) error {
	return rdb.Watch(ctx, func(tx *redis.Tx) error {
		_, err := rdb.TxPipelined(ctx, func(pipe redis.Pipeliner) error { // want `^rdb bypasses the transaction opened with Watch; use tx instead \[outer-handle\]$`
			return nil
		})
		return err
	})
}

func Cmdable(ctx context.Context, c redis.Cmdable) error {
	_, err := c.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		c.Set(ctx, "a", 1, 0) // want `^c bypasses the transaction opened with TxPipelined; use pipe instead \[outer-handle\]$`
		return nil
	})
	return err
}

func V8(ctx context.Context, rdb *redis8.Client) error {
	_, err := rdb.TxPipelined(ctx, func(pipe redis8.Pipeliner) error {
		rdb.Set(ctx, "a", 1, 0) // want `^rdb bypasses the transaction opened with TxPipelined; use pipe instead \[outer-handle\]$`
		return nil
	})
	return err
}
