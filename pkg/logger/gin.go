package logger

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strconv"
	"time"

	"log/slog"

	"github.com/gin-gonic/gin"
)

func init() {
	gin.ForceConsoleColor()
}

func GinLogger(lg *slog.Logger) gin.HandlerFunc {
	v, ok := os.LookupEnv("OPNI_ENABLE_GIN_LOGGING")
	enabled, _ := strconv.ParseBool(v)
	if ok && enabled {
		r, w := io.Pipe()
		go func() {
			// read log entries from r, and log using lg
			scanner := bufio.NewScanner(r)
			for scanner.Scan() {
				lg.Debug(scanner.Text())
			}
		}()
		return gin.LoggerWithConfig(gin.LoggerConfig{
			SkipPaths: []string{"/healthz"},
			Formatter: logFormatter,
			Output:    w,
		})
	}
	return gin.LoggerWithWriter(io.Discard)
}

func logFormatter(params gin.LogFormatterParams) string {
	var statusColor, methodColor, resetColor string
	if params.IsOutputColor() {
		statusColor = params.StatusCodeColor()
		methodColor = params.MethodColor()
		resetColor = params.ResetColor()
	}

	if params.Latency > time.Minute {
		params.Latency = params.Latency.Truncate(time.Second)
	}
	newline := "\n"
	if len(params.ErrorMessage) > 0 && params.ErrorMessage[len(params.ErrorMessage)-1] == '\n' {
		newline = ""
	}
	return fmt.Sprintf("%s %s %s %#v |%s %3d %s| %13v | %15s | %s%s",
		methodColor, params.Method, resetColor,
		params.Path,
		statusColor, params.StatusCode, resetColor,
		params.Latency,
		params.ClientIP,
		params.ErrorMessage,
		newline,
	)
}
