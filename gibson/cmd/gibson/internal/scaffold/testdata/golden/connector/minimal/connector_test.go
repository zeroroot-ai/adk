package connector

import (
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// entry mirrors the fields that gibson's platform catalog parses from a
// connector manifest (internal/platform/componentcatalog, ADR-0065 R6). It is
// declared locally so this smoke test stays hermetic and never imports gibson.
type entry struct {
	ID          string   `yaml:"id"`
	Kind        string   `yaml:"kind"`
	DisplayName string   `yaml:"displayName"`
	Description string   `yaml:"description"`
	EgressAllow []string `yaml:"egressAllow"`
	Spec        spec     `yaml:"spec"`
}

// spec is the connector block of a catalog entry.
type spec struct {
	Vendor     string `yaml:"vendor"`
	Shape      string `yaml:"shape"`
	Image      string `yaml:"image"`
	Endpoint   string `yaml:"endpoint"`
	Transport  string `yaml:"transport"`
	Auth       string `yaml:"auth"`
	OAuthScope string `yaml:"oauthScope"`
}

// validate applies the same shape invariants gibson's catalog loader enforces,
// so a malformed manifest fails here — in the connector's own CI — long before
// gibson ever loads it.
func (e entry) validate() error {
	if e.ID == "" {
		return errInvalid("id is required")
	}
	if e.Kind != "connector" {
		return errInvalid("kind must be connector, got " + e.Kind)
	}
	if e.Spec.Transport == "" {
		return errInvalid("transport is required")
	}
	switch e.Spec.Shape {
	case "Remote":
		if e.Spec.Endpoint == "" {
			return errInvalid("a Remote connector needs an endpoint")
		}
		if e.Spec.Image != "" {
			return errInvalid("a Remote connector must not set image")
		}
	case "Hosted":
		if e.Spec.Image == "" {
			return errInvalid("a Hosted connector needs an image")
		}
		if e.Spec.Endpoint != "" {
			return errInvalid("a Hosted connector must not set endpoint")
		}
	default:
		return errInvalid("shape must be Hosted or Remote, got " + e.Spec.Shape)
	}
	switch e.Spec.Auth {
	case "none", "secret", "oauth":
	default:
		return errInvalid("auth must be none, secret, or oauth, got " + e.Spec.Auth)
	}
	return nil
}

type errInvalid string

func (e errInvalid) Error() string { return string(e) }

func TestConnectorManifest(t *testing.T) {
	raw, err := os.ReadFile("connector.yaml")
	if err != nil {
		t.Fatalf("read connector.yaml: %v", err)
	}
	var e entry
	if err := yaml.Unmarshal(raw, &e); err != nil {
		t.Fatalf("parse connector.yaml: %v", err)
	}
	if err := e.validate(); err != nil {
		t.Fatalf("connector.yaml is invalid: %v", err)
	}
}
