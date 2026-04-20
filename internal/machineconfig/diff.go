package machineconfig

import (
	"bytes"
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// NormalizeYAML returns a consistently marshaled YAML representation.
func NormalizeYAML(data []byte) ([]byte, error) {
	var value any
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, fmt.Errorf("parsing YAML: %w", err)
	}
	normalized, err := yaml.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("normalizing YAML: %w", err)
	}
	return bytes.TrimSpace(normalized), nil
}

// DiffFiles compares two YAML files after normalization.
func DiffFiles(leftPath, rightPath string) ([]string, bool, error) {
	leftRaw, err := os.ReadFile(leftPath)
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", leftPath, err)
	}
	rightRaw, err := os.ReadFile(rightPath)
	if err != nil {
		return nil, false, fmt.Errorf("reading %s: %w", rightPath, err)
	}
	left, err := NormalizeYAML(leftRaw)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", leftPath, err)
	}
	right, err := NormalizeYAML(rightRaw)
	if err != nil {
		return nil, false, fmt.Errorf("%s: %w", rightPath, err)
	}
	if bytes.Equal(left, right) {
		return nil, false, nil
	}
	return lineDiff(string(left), string(right)), true, nil
}

func lineDiff(left, right string) []string {
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	max := len(leftLines)
	if len(rightLines) > max {
		max = len(rightLines)
	}
	out := []string{}
	for i := 0; i < max; i++ {
		var l, r string
		if i < len(leftLines) {
			l = leftLines[i]
		}
		if i < len(rightLines) {
			r = rightLines[i]
		}
		if l == r {
			continue
		}
		if l != "" {
			out = append(out, "- "+l)
		}
		if r != "" {
			out = append(out, "+ "+r)
		}
	}
	return out
}
