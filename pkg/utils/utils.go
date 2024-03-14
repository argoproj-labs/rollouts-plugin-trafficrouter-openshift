package utils

import (
	"os"

	"log/slog"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func NewKubeConfig() (*rest.Config, error) {
	rules := clientcmd.NewDefaultClientConfigLoadingRules()
	config := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, &clientcmd.ConfigOverrides{})
	return config.ClientConfig()
}

func InitLogger(lvl slog.Level) {
	lvlVar := &slog.LevelVar{}
	lvlVar.Set(lvl)
	opts := slog.HandlerOptions{
		Level: lvlVar,
	}

	attrs := []slog.Attr{
		slog.String("plugin", "trafficrouter"),
		slog.String("vendor", "openshift"),
	}

	l := slog.New(slog.NewTextHandler(os.Stderr, &opts).WithAttrs(attrs))
	slog.SetDefault(l)
}
