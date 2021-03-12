package pubsub

import (
	"github.com/gin-gonic/gin"
	"github.com/pkg/errors"
)

func mustWriteString(c *gin.Context, s string) {
	if _, err := c.Writer.WriteString(s); err != nil {
		panic(errors.Wrap(err, "Failed to write output"))
	}
}
