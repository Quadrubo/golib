package grpcserver

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"google.golang.org/grpc/grpclog"

	"github.com/quadrubo/golib/logging"
)

func GRPCLogger(log *slog.Logger) grpclog.LoggerV2 {
	return &grpcLogger{log: log.With(slog.String(logging.ComponentKey, "grpc"))}
}

// grpcLogger routes gRPC's own logging into slog. Its severities are fixed, so
// they map onto slog levels rather than the configured one.
type grpcLogger struct {
	log *slog.Logger
}

func (g *grpcLogger) Info(args ...any) { g.write(slog.LevelInfo, fmt.Sprint(args...)) }
func (g *grpcLogger) Infoln(args ...any) {
	g.write(slog.LevelInfo, fmt.Sprintln(args...))
}

func (g *grpcLogger) Infof(format string, args ...any) {
	g.write(slog.LevelInfo, fmt.Sprintf(format, args...))
}

func (g *grpcLogger) Warning(args ...any) { g.write(slog.LevelWarn, fmt.Sprint(args...)) }
func (g *grpcLogger) Warningln(args ...any) {
	g.write(slog.LevelWarn, fmt.Sprintln(args...))
}

func (g *grpcLogger) Warningf(format string, args ...any) {
	g.write(slog.LevelWarn, fmt.Sprintf(format, args...))
}

func (g *grpcLogger) Error(args ...any) { g.write(slog.LevelError, fmt.Sprint(args...)) }
func (g *grpcLogger) Errorln(args ...any) {
	g.write(slog.LevelError, fmt.Sprintln(args...))
}

func (g *grpcLogger) Errorf(format string, args ...any) {
	g.write(slog.LevelError, fmt.Sprintf(format, args...))
}

// grpc expects the process to be gone when Fatal returns.
func (g *grpcLogger) Fatal(args ...any) {
	g.write(slog.LevelError, fmt.Sprint(args...))
	os.Exit(1)
}

func (g *grpcLogger) Fatalln(args ...any) {
	g.write(slog.LevelError, fmt.Sprintln(args...))
	os.Exit(1)
}

func (g *grpcLogger) Fatalf(format string, args ...any) {
	g.write(slog.LevelError, fmt.Sprintf(format, args...))
	os.Exit(1)
}

// Verbosity 0, matching grpc's own default when GRPC_GO_LOG_VERBOSITY_LEVEL is
// unset.
func (g *grpcLogger) V(l int) bool { return l <= 0 }

func (g *grpcLogger) write(level slog.Level, msg string) {
	g.log.Log(context.Background(), level, strings.TrimSuffix(msg, "\n"))
}
