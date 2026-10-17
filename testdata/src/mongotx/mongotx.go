// Package mongotx covers outer-context with MongoDB, whose transaction rides
// in the callback's context.
package mongotx

import (
	"context"

	mongo1 "go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type User struct{ Name string }

// ===== Reported =====

func WithTransaction(ctx context.Context, sess *mongo.Session, coll *mongo.Collection, u *User) error {
	_, err := sess.WithTransaction(ctx, func(sc context.Context) (any, error) {
		if _, err := coll.InsertOne(sc, u); err != nil {
			return nil, err
		}
		return coll.InsertOne(ctx, u) // want `^ctx bypasses the transaction opened with WithTransaction; use sc instead \[outer-context\]$`
	})
	return err
}

func WithSession(ctx context.Context, sess *mongo.Session, coll *mongo.Collection, u *User) error {
	return mongo.WithSession(ctx, sess, func(_ context.Context) error {
		_, err := coll.InsertOne(ctx, u) // want `^ctx bypasses the transaction opened with mongo\.WithSession; use the callback's context instead \[outer-context\]$`
		return err
	})
}

func V1(ctx context.Context, sess mongo1.Session, coll *mongo1.Collection, u *User) error {
	_, err := sess.WithTransaction(ctx, func(sc mongo1.SessionContext) (interface{}, error) {
		return coll.InsertOne(ctx, u) // want `^ctx bypasses the transaction opened with WithTransaction; use sc instead \[outer-context\]$`
	})
	if err != nil {
		return err
	}
	return mongo1.WithSession(ctx, sess, func(sc mongo1.SessionContext) error {
		_, err := coll.InsertOne(ctx, u) // want `^ctx bypasses the transaction opened with mongo\.WithSession; use sc instead \[outer-context\]$`
		return err
	})
}

// ===== Not reported =====

// The callback's context shadows the outer one.
func Shadowed(ctx context.Context, sess *mongo.Session, coll *mongo.Collection, u *User) error {
	_, err := sess.WithTransaction(ctx, func(ctx context.Context) (any, error) {
		return coll.InsertOne(ctx, u)
	})
	return err
}

// The collection is not the handle: the context decides the transaction.
func Collection(ctx context.Context, sess *mongo.Session, coll *mongo.Collection, u *User) error {
	_, err := sess.WithTransaction(ctx, func(sc context.Context) (any, error) {
		return coll.InsertOne(sc, u)
	})
	return err
}

// A context given by a call names no variable.
func Fresh(sess *mongo.Session, coll *mongo.Collection, u *User) error {
	_, err := sess.WithTransaction(context.Background(), func(sc context.Context) (any, error) {
		return coll.InsertOne(sc, u)
	})
	return err
}
