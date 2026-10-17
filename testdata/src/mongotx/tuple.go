package mongotx

import (
	"context"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func sessionArgs() (context.Context, *mongo.Session, func(context.Context) error) {
	return nil, nil, nil
}

// Not an opener here, and no crash: the arguments are one call with several
// results.
func Tuple() error {
	return mongo.WithSession(sessionArgs())
}
