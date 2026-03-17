package rules

import (
	"fmt"
	"sort"

	"github.com/OneNoted/pvt/internal/proxmox"
)

// Severity indicates how critical a validation finding is.
type Severity int

const (
	SeverityInfo Severity = iota
	SeverityWarn
	SeverityError
)

func (s Severity) String() string {
	switch s {
	case SeverityInfo:
		return "INFO"
	case SeverityWarn:
		return "WARN"
	case SeverityError:
		return "ERROR"
	default:
		return "UNKNOWN"
	}
}

// Finding represents a single validation result.
type Finding struct {
	Rule     string
	Severity Severity
	Message  string
	Fix      string // Suggested qm set command or API call
	Current  string // Current value
	Expected string // Expected value
}

// Rule defines a single validation check against a VM configuration.
type Rule interface {
	// Name returns the rule identifier (e.g. "cpu-type").
	Name() string
	// Description returns a human-readable description of what the rule checks.
	Description() string
	// DefaultSeverity returns the default severity for this rule.
	DefaultSeverity() Severity
	// Check evaluates the rule against a VM config and returns any findings.
	Check(vm *proxmox.VMConfig) []Finding
}

// Registry holds all registered validation rules.
type Registry struct {
	rules map[string]Rule
}

// NewRegistry creates a new empty rule registry.
func NewRegistry() *Registry {
	return &Registry{
		rules: make(map[string]Rule),
	}
}

// Register adds a rule to the registry.
func (r *Registry) Register(rule Rule) error {
	name := rule.Name()
	if _, exists := r.rules[name]; exists {
		return fmt.Errorf("rule %q already registered", name)
	}
	r.rules[name] = rule
	return nil
}

// Get returns a rule by name.
func (r *Registry) Get(name string) (Rule, bool) {
	rule, ok := r.rules[name]
	return rule, ok
}

// All returns all registered rules sorted by name.
func (r *Registry) All() []Rule {
	result := make([]Rule, 0, len(r.rules))
	for _, rule := range r.rules {
		result = append(result, rule)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Name() < result[j].Name()
	})
	return result
}

// Validate runs all rules against a VM config and returns all findings.
func (r *Registry) Validate(vm *proxmox.VMConfig) []Finding {
	var findings []Finding
	for _, rule := range r.All() {
		findings = append(findings, rule.Check(vm)...)
	}
	return findings
}

// DefaultRegistry creates a registry with all built-in rules.
func DefaultRegistry() *Registry {
	reg := NewRegistry()

	// Register all built-in rules
	builtins := []Rule{
		&CPUTypeRule{},
		&SCSIHWRule{},
		&MemoryMinRule{MinMiB: 2048},
		&BalloonRule{},
		&NetworkModelRule{},
		&QEMUAgentRule{},
		&MachineTypeRule{},
		&SerialConsoleRule{},
	}

	for _, rule := range builtins {
		_ = reg.Register(rule) // built-in rules have unique names by construction
	}

	return reg
}
