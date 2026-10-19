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
	"github.com/mpyw/txbypass/internal"
	"github.com/mpyw/txbypass/internal/directive"
	"github.com/mpyw/txbypass/internal/funcname"
	"github.com/mpyw/txbypass/internal/rule"
)

func init() {
	register.Plugin(txbypass.Analyzer.Name, newPlugin)
}

// pluginRules is the plugin built from one settings block.
type pluginRules struct {
	// on holds the rules the settings name.
	on map[rule.Name]bool
	// declared holds the functions the settings declare, by kind.
	declared map[directive.Kind]map[string]bool
}

func newPlugin(settings any) (register.LinterPlugin, error) {
	named, err := register.DecodeSettings[map[string]any](settings)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	p := &pluginRules{on: make(map[rule.Name]bool), declared: make(map[directive.Kind]map[string]bool)}
	for name, v := range named {
		switch kind := directive.Kind(name); kind {
		case directive.KindInvokes, directive.KindBegins, directive.KindCloses:
			names, err := pluginNames(v)
			if err != nil {
				return nil, fmt.Errorf("%s in settings: %w", name, err)
			}
			p.declared[kind] = names
			continue
		}
		if !rule.Known(name) {
			return nil, fmt.Errorf("unknown rule %q in settings", name)
		}
		on, ok := v.(bool)
		if !ok {
			return nil, fmt.Errorf("rule %q in settings takes true or false", name)
		}
		p.on[rule.Name(name)] = on
	}
	return p, nil
}

// pluginNames reads a list of functions, one name per item.
func pluginNames(v any) (map[string]bool, error) {
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
	return funcname.ParseEach(names)
}

// BuildAnalyzers gives txbypass's analyzer, with the rules the settings turn on.
// It is a copy, so the flags of txbypass.Analyzer are left alone.
func (p *pluginRules) BuildAnalyzers() ([]*analysis.Analyzer, error) {
	cfg := internal.Config{On: p.ruleOn, Declared: p.declared}
	return []*analysis.Analyzer{{
		Name:      txbypass.Analyzer.Name,
		Doc:       txbypass.Analyzer.Doc,
		URL:       txbypass.Analyzer.URL,
		Requires:  txbypass.Analyzer.Requires,
		FactTypes: txbypass.Analyzer.FactTypes,
		Run: func(pass *analysis.Pass) (any, error) {
			return internal.Run(pass, cfg)
		},
	}}, nil
}

// GetLoadMode asks for type information, which the rules read.
func (*pluginRules) GetLoadMode() string {
	return register.LoadModeTypesInfo
}

func (p *pluginRules) ruleOn(r rule.Name) bool {
	if v, ok := p.on[r]; ok {
		return v
	}
	return rule.OnByDefault(r)
}
