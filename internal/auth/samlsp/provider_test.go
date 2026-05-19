// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package samlsp

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/crewjam/saml"
)

func TestResolveRole(t *testing.T) {
	tests := []struct {
		name        string
		roleMapping map[string]string
		groups      []string
		want        string
	}{
		{
			name:        "no mapping defaults to viewer",
			roleMapping: nil,
			groups:      []string{"eng", "ops"},
			want:        "viewer",
		},
		{
			name:        "empty groups defaults to viewer",
			roleMapping: map[string]string{"admins": "admin"},
			groups:      nil,
			want:        "viewer",
		},
		{
			name:        "single group matches",
			roleMapping: map[string]string{"ops": "operator"},
			groups:      []string{"ops"},
			want:        "operator",
		},
		{
			name:        "highest priority wins",
			roleMapping: map[string]string{"viewers": "viewer", "ops": "operator", "admins": "admin"},
			groups:      []string{"viewers", "ops", "admins"},
			want:        "admin",
		},
		{
			name:        "operator beats viewer",
			roleMapping: map[string]string{"readers": "viewer", "ops": "operator"},
			groups:      []string{"readers", "ops"},
			want:        "operator",
		},
		{
			name:        "no matching group defaults to viewer",
			roleMapping: map[string]string{"admins": "admin"},
			groups:      []string{"eng", "design"},
			want:        "viewer",
		},
		{
			name:        "unknown role in mapping is ignored",
			roleMapping: map[string]string{"group1": "superadmin"},
			groups:      []string{"group1"},
			want:        "viewer",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &Provider{cfg: Config{RoleMapping: tt.roleMapping}}
			got := p.resolveRole(tt.groups)
			if got != tt.want {
				t.Errorf("resolveRole(%v) = %q, want %q", tt.groups, got, tt.want)
			}
		})
	}
}

func TestBuildSAMLSubject(t *testing.T) {
	tests := []struct {
		idpEntityID string
		nameID      string
		want        string
	}{
		{"https://idp.example.com", "user@example.com", "https://idp.example.com:user@example.com"},
		{"", "anon", ":anon"},
		{"urn:idp", "uid=123", "urn:idp:uid=123"},
	}

	for _, tt := range tests {
		got := buildSAMLSubject(tt.idpEntityID, tt.nameID)
		if got != tt.want {
			t.Errorf("buildSAMLSubject(%q, %q) = %q, want %q",
				tt.idpEntityID, tt.nameID, got, tt.want)
		}
	}
}

func TestFlattenAttributes(t *testing.T) {
	assertion := &saml.Assertion{
		AttributeStatements: []saml.AttributeStatement{
			{
				Attributes: []saml.Attribute{
					{
						Name:         "urn:oid:0.9.2342.19200300.100.1.3",
						FriendlyName: "mail",
						Values: []saml.AttributeValue{
							{Value: "user@example.com"},
						},
					},
					{
						Name: "groups",
						Values: []saml.AttributeValue{
							{Value: "eng"},
							{Value: "ops"},
						},
					},
				},
			},
		},
	}

	attrs := flattenAttributes(assertion)

	// Check OID-keyed attribute.
	if v, ok := attrs["urn:oid:0.9.2342.19200300.100.1.3"]; !ok || v[0] != "user@example.com" {
		t.Errorf("expected email via OID, got %v", attrs["urn:oid:0.9.2342.19200300.100.1.3"])
	}

	// Check FriendlyName alias.
	if v, ok := attrs["mail"]; !ok || v[0] != "user@example.com" {
		t.Errorf("expected email via FriendlyName, got %v", attrs["mail"])
	}

	// Check multi-value attribute.
	if v, ok := attrs["groups"]; !ok || len(v) != 2 || v[0] != "eng" || v[1] != "ops" {
		t.Errorf("expected groups [eng, ops], got %v", attrs["groups"])
	}
}

func TestExtractUserInfo(t *testing.T) {
	p := &Provider{
		cfg: Config{
			UsernameAttr:    "uid",
			EmailAttr:       "mail",
			DisplayNameAttr: "cn",
			GroupsAttr:      "memberOf",
			RoleMapping:     map[string]string{"ops-team": "operator"},
		},
		idpMetadata: &saml.EntityDescriptor{EntityID: "https://idp.example.com"},
		logger:      func(string, string) {},
	}

	assertion := &saml.Assertion{
		Subject: &saml.Subject{
			NameID: &saml.NameID{Value: "nameid-123"},
		},
		AttributeStatements: []saml.AttributeStatement{
			{
				Attributes: []saml.Attribute{
					{Name: "uid", Values: []saml.AttributeValue{{Value: "jdoe"}}},
					{Name: "mail", Values: []saml.AttributeValue{{Value: "jdoe@example.com"}}},
					{Name: "cn", Values: []saml.AttributeValue{{Value: "Jane Doe"}}},
					{Name: "memberOf", Values: []saml.AttributeValue{{Value: "ops-team"}, {Value: "eng"}}},
				},
			},
		},
	}

	info := p.extractUserInfo(assertion)

	if info.SAMLSubject != "https://idp.example.com:nameid-123" {
		t.Errorf("SAMLSubject = %q, want %q", info.SAMLSubject, "https://idp.example.com:nameid-123")
	}
	if info.Username != "jdoe" {
		t.Errorf("Username = %q, want %q", info.Username, "jdoe")
	}
	if info.Email != "jdoe@example.com" {
		t.Errorf("Email = %q, want %q", info.Email, "jdoe@example.com")
	}
	if info.DisplayName != "Jane Doe" {
		t.Errorf("DisplayName = %q, want %q", info.DisplayName, "Jane Doe")
	}
	if info.Role != "operator" {
		t.Errorf("Role = %q, want %q", info.Role, "operator")
	}
	if len(info.Groups) != 2 {
		t.Errorf("Groups = %v, want 2 items", info.Groups)
	}
}

func TestExtractUserInfo_FallbackToNameID(t *testing.T) {
	p := &Provider{
		cfg:         Config{},
		idpMetadata: &saml.EntityDescriptor{EntityID: "https://idp.example.com"},
		logger:      func(string, string) {},
	}

	assertion := &saml.Assertion{
		Subject: &saml.Subject{
			NameID: &saml.NameID{Value: "user@example.com"},
		},
		AttributeStatements: []saml.AttributeStatement{},
	}

	info := p.extractUserInfo(assertion)

	if info.Username != "user@example.com" {
		t.Errorf("Username should fall back to NameID, got %q", info.Username)
	}
	if info.Role != "viewer" {
		t.Errorf("Role should default to viewer, got %q", info.Role)
	}
}

func TestRequestStore(t *testing.T) {
	store := newRequestStore(100 * time.Millisecond)

	store.Store("req-1")
	store.Store("req-2")

	ids := store.possibleRequestIDs()
	if len(ids) != 2 {
		t.Fatalf("expected 2 IDs, got %d", len(ids))
	}

	store.Delete("req-1")
	ids = store.possibleRequestIDs()
	if len(ids) != 1 {
		t.Fatalf("expected 1 ID after delete, got %d", len(ids))
	}

	// Wait for TTL expiry.
	time.Sleep(150 * time.Millisecond)
	ids = store.possibleRequestIDs()
	if len(ids) != 0 {
		t.Fatalf("expected 0 IDs after TTL, got %d", len(ids))
	}
}

func TestRequestStore_Concurrent(t *testing.T) {
	store := newRequestStore(5 * time.Second)

	done := make(chan struct{})
	for i := 0; i < 100; i++ {
		go func(n int) {
			store.Store(string(rune('A' + n%26)))
			_ = store.possibleRequestIDs()
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 100; i++ {
		<-done
	}
}

func TestNewProvider_ValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{
			name:    "missing idp_metadata_url",
			cfg:     Config{SPEntityID: "x", ACSURL: "x", Certificate: []byte("x"), PrivateKey: []byte("x")},
			wantErr: "idp_metadata_url is required",
		},
		{
			name:    "missing sp_entity_id",
			cfg:     Config{IDPMetadataURL: "https://idp.example.com/metadata", ACSURL: "x", Certificate: []byte("x"), PrivateKey: []byte("x")},
			wantErr: "sp_entity_id is required",
		},
		{
			name:    "missing acs_url",
			cfg:     Config{IDPMetadataURL: "https://idp.example.com/metadata", SPEntityID: "x", Certificate: []byte("x"), PrivateKey: []byte("x")},
			wantErr: "acs_url is required",
		},
		{
			name:    "missing certificate",
			cfg:     Config{IDPMetadataURL: "https://idp.example.com/metadata", SPEntityID: "x", ACSURL: "x", PrivateKey: []byte("x")},
			wantErr: "certificate is required",
		},
		{
			name:    "missing private_key",
			cfg:     Config{IDPMetadataURL: "https://idp.example.com/metadata", SPEntityID: "x", ACSURL: "x", Certificate: []byte("x")},
			wantErr: "private_key is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.cfg)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestParseCertAndKey_Invalid(t *testing.T) {
	_, _, err := parseCertAndKey([]byte("not-a-cert"), []byte("not-a-key"))
	if err == nil {
		t.Fatal("expected error for invalid PEM")
	}
}

func TestDecodeLogoutRequest_Base64XML(t *testing.T) {
	// Minimal LogoutRequest XML.
	xmlStr := `<LogoutRequest xmlns="urn:oasis:names:tc:SAML:2.0:protocol" ID="req-123" Version="2.0">
		<NameID xmlns="urn:oasis:names:tc:SAML:2.0:assertion">user@example.com</NameID>
	</LogoutRequest>`

	// POST binding: plain base64.
	encoded := base64.StdEncoding.EncodeToString([]byte(xmlStr))

	req, err := decodeLogoutRequest(encoded)
	if err != nil {
		t.Fatalf("decodeLogoutRequest: %v", err)
	}
	if req.ID != "req-123" {
		t.Errorf("ID = %q, want %q", req.ID, "req-123")
	}
	if req.NameID == nil || req.NameID.Value != "user@example.com" {
		t.Errorf("NameID = %v, want user@example.com", req.NameID)
	}
}
