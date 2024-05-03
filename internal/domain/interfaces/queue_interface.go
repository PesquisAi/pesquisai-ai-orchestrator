package interfaces

import (
	"context"
)

type Queue interface {
	Publish(ctx context.Context, b []byte) (err error)
	Connect(queueName string) (err error)
}