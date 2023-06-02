package shipper

import (
	"bufio"
	"context"
)

// Shipper is a convenient interface for publishing data to a destination
type Shipper interface {
	// Publish will publish each token in a scanner to a destination
	// any batch processing should be handled by the shipper
	Publish(context.Context, *bufio.Scanner) error
}
