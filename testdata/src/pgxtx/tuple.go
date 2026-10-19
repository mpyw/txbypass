package pgxtx

import (
	"context"

	"github.com/jackc/pgx/v5"
)

func beginArgs() (context.Context, *pgx.Conn, func(pgx.Tx) error) { return nil, nil, nil }

// Not an invoker here, and no crash: the arguments are one call with several
// results, so there is no function literal to read.
func Tuple() error {
	return pgx.BeginFunc(beginArgs())
}
