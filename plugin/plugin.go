// Package plugin registers txbypass as a golangci-lint module plugin.
//
// Import it from .custom-gcl.yml to build a golangci-lint binary that holds
// txbypass. The settings in .golangci.yml map each rule's name to whether it is
// on, as the flags of the txbypass command do. A rule left out keeps its
// default. invokes, begins and closes each take a list of functions, one per
// item.
package plugin

import (
	"fmt"

	"github.com/golangci/plugin-module-register/register"
	"golang.org/x/tools/go/analysis"

	"github.com/mpyw/txbypass"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/rule"
)

func init() {
	register.Plugin(txbypass.Analyzer.Name, newPlugin)
}

// pluginAnalyzer is the plugin built from one settings block.
type pluginAnalyzer struct {
	analyzer *analysis.Analyzer
}

func newPlugin(settings any) (register.LinterPlugin, error) {
	named, err := register.DecodeSettings[map[string]any](settings)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	cfg := txbypass.Config{Rules: make(map[string]bool)}
	lists := map[directive.Kind]*[]string{
		directive.KindInvokes: &cfg.Invokes,
		directive.KindBegins:  &cfg.Begins,
		directive.KindCloses:  &cfg.Closes,
	}
	for name, v := range named {
		if list, ok := lists[directive.Kind(name)]; ok {
			names, err := pluginNames(v)
			if err != nil {
				return nil, fmt.Errorf("%s in settings: %w", name, err)
			}
			*list = names
			continue
		}
		if !rule.Known(name) {
			return nil, fmt.Errorf("unknown rule %q in settings", name)
		}
		on, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("rule %q in settings takes true or false", name)
		}
		cfg.Rules[name] = on
	}
	a, err := txbypass.New(cfg)
	if err != nil {
		return nil, fmt.Errorf("settings: %w", err)
	}
	return &pluginAnalyzer{analyzer: a}, nil
}

// pluginNames reads a list of functions, one name per item.
func pluginNames(v any) ([]string, error) {
	items, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("takes a list of functions")
	}
	names := make([]string, len(items))
	for i, item := range items {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("takes a list of functions")
		}
		names[i] = s
	}
	return names, nil
}

// BuildAnalyzers gives the analyzer that txbypass.New built from the
// settings. It has no flags, so txbypass.Analyzer is left alone.
func (p *pluginAnalyzer) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	return []*analysis.Analyzer{p.analyzer}, nil
}

// GetLoadMode asks for type information, which the rules read.
func (*pluginAnalyzer) GetLoadMode() string {
	return register.LoadModeTypesInfo
}
