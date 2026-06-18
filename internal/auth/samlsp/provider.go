// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

// Package samlsp implements the SAML 2.0 Service Provider for Chef Migration
// Metrics. It wraps the crewjam/saml library to provide SP-initiated SSO,
// assertion validation, attribute extraction, group-to-role mapping, and
// inbound Single Logout.
package samlsp

import (
	"compress/flate"
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	dsig "github.com/russellhaering/goxmldsig"
)

// Config holds the SAML SP configuration needed to initialise the provider.
type Config struct {
	// IDPMetadataURL is the HTTPS URL to fetch IdP metadata from.
	// Mutually exclusive with IDPMetadataPath.
	IDPMetadataURL string

	// IDPMetadataPath is a local file path to IdP metadata XML.
	// Used when the IdP doesn't expose a fetchable metadata URL (e.g. Google Workspace).
	// Mutually exclusive with IDPMetadataURL.
	IDPMetadataPath string

	// IDPMetadataXML is the raw IdP metadata XML pasted directly into the
	// configuration. Used when the IdP exposes neither a fetchable URL nor a
	// file the server can read. Mutually exclusive with IDPMetadataURL and
	// IDPMetadataPath.
	IDPMetadataXML []byte

	// SPEntityID is the entity ID of this service provider.
	SPEntityID string

	// ACSURL is the full URL of the assertion consumer service endpoint.
	ACSURL string

	// SLOURL is the full URL of the single logout endpoint.
	SLOURL string

	// MetadataURL is the full URL where SP metadata is served.
	MetadataURL string

	// Certificate is the PEM-encoded SP signing certificate.
	Certificate []byte

	// PrivateKey is the PEM-encoded SP signing private key.
	PrivateKey []byte

	// UsernameAttr is the SAML attribute name for the username.
	// If empty, NameID is used.
	UsernameAttr string

	// EmailAttr is the SAML attribute name for the email.
	EmailAttr string

	// DisplayNameAttr is the SAML attribute name for the display name.
	DisplayNameAttr string

	// GroupsAttr is the SAML attribute name for group membership.
	GroupsAttr string

	// RoleAttr is a SAML attribute name that contains the role directly
	// (e.g. "admin", "operator", "viewer"). If set AND the attribute is
	// present in the assertion, it takes precedence over group-based mapping.
	RoleAttr string

	// RoleMapping maps SAML group names to application roles.
	RoleMapping map[string]string

	// AllowIDPInitiated controls whether unsolicited responses are accepted.
	AllowIDPInitiated bool

	// SignRequests controls whether AuthnRequests are signed.
	SignRequests bool

	// DebugLogAssertions, when true, logs the full decrypted assertion XML at
	// the ACS point (at WARN, since the XML contains PII and a replayable
	// credential). Off by default; intended as a short-lived diagnostic toggle.
	DebugLogAssertions bool

	// ClockSkewTolerance is the maximum clock skew allowed for assertion
	// validation. Zero means use the default (5 minutes).
	ClockSkewTolerance time.Duration

	// MetadataRefreshInterval controls how often IdP metadata is refreshed.
	// Zero means use the default (24 hours).
	MetadataRefreshInterval time.Duration

	// Logger is an optional callback for logging events.
	Logger func(level, msg string)
}

// UserInfo holds the identity information extracted from a SAML assertion.
type UserInfo struct {
	// SAMLSubject is the stable identity key: "{idp_entity_id}:{NameID}".
	SAMLSubject string

	// Username is the extracted username (from attribute or NameID).
	Username string

	// Email is the extracted email address.
	Email string

	// DisplayName is the extracted display name.
	DisplayName string

	// Groups is the list of group memberships from the assertion.
	Groups []string

	// Role is the application role resolved from group membership.
	Role string
}

// Provider is the SAML Service Provider. It handles metadata serving,
// SSO initiation, assertion processing, and inbound SLO.
type Provider struct {
	sp     saml.ServiceProvider
	cfg    Config
	logger func(level, msg string)

	// requestStore holds pending AuthnRequest IDs for InResponseTo validation.
	requestStore *requestStore

	// metadataMu guards idpMetadata for concurrent refresh.
	metadataMu  sync.RWMutex
	idpMetadata *saml.EntityDescriptor
}

// New creates a new SAML Provider from the given configuration.
func New(cfg Config) (*Provider, error) {
	if cfg.IDPMetadataURL == "" && cfg.IDPMetadataPath == "" && len(cfg.IDPMetadataXML) == 0 {
		return nil, errors.New("samlsp: idp_metadata_url, idp_metadata_path, or idp_metadata_xml is required")
	}
	if cfg.SPEntityID == "" {
		return nil, errors.New("samlsp: sp_entity_id is required")
	}
	if cfg.ACSURL == "" {
		return nil, errors.New("samlsp: acs_url is required")
	}
	if len(cfg.Certificate) == 0 {
		return nil, errors.New("samlsp: certificate is required")
	}
	if len(cfg.PrivateKey) == 0 {
		return nil, errors.New("samlsp: private_key is required")
	}

	// Parse the SP certificate and key.
	cert, key, err := parseCertAndKey(cfg.Certificate, cfg.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("samlsp: %w", err)
	}

	// Parse URLs.
	acsURL, err := url.Parse(cfg.ACSURL)
	if err != nil {
		return nil, fmt.Errorf("samlsp: invalid acs_url: %w", err)
	}
	metadataURL, _ := url.Parse(cfg.MetadataURL)
	sloURL, _ := url.Parse(cfg.SLOURL)

	// Load IdP metadata from inline XML, file, or URL (in that precedence).
	var idpMeta *saml.EntityDescriptor
	switch {
	case len(cfg.IDPMetadataXML) > 0:
		idpMeta, err = loadIDPMetadataFromXML(cfg.IDPMetadataXML)
	case cfg.IDPMetadataPath != "":
		idpMeta, err = loadIDPMetadataFromFile(cfg.IDPMetadataPath)
	default:
		idpMeta, err = fetchIDPMetadata(cfg.IDPMetadataURL)
	}
	if err != nil {
		return nil, fmt.Errorf("samlsp: loading idp metadata: %w", err)
	}

	// Build the crewjam/saml ServiceProvider.
	sp := saml.ServiceProvider{
		EntityID:          cfg.SPEntityID,
		Key:               key,
		Certificate:       cert,
		MetadataURL:       *metadataURL,
		AcsURL:            *acsURL,
		SloURL:            *sloURL,
		IDPMetadata:       idpMeta,
		AllowIDPInitiated: cfg.AllowIDPInitiated,
	}

	// Sign requests using SHA-256 if configured.
	if cfg.SignRequests {
		sp.SignatureMethod = dsig.RSASHA256SignatureMethod
	}

	if cfg.ClockSkewTolerance > 0 {
		saml.MaxClockSkew = cfg.ClockSkewTolerance
	}

	logger := cfg.Logger
	if logger == nil {
		logger = func(string, string) {}
	}

	p := &Provider{
		sp:           sp,
		cfg:          cfg,
		logger:       logger,
		requestStore: newRequestStore(10 * time.Minute),
		idpMetadata:  idpMeta,
	}

	return p, nil
}

// Metadata returns the SP metadata XML bytes.
func (p *Provider) Metadata() ([]byte, error) {
	meta := p.sp.Metadata()
	return xml.MarshalIndent(meta, "", "  ")
}

// MakeAuthnRequest generates an AuthnRequest and returns the IdP redirect URL.
// The request ID is stored for later InResponseTo validation.
func (p *Provider) MakeAuthnRequest(relayState string) (*url.URL, error) {
	p.metadataMu.RLock()
	defer p.metadataMu.RUnlock()

	authReq, err := p.sp.MakeAuthenticationRequest(
		p.sp.GetSSOBindingLocation(saml.HTTPRedirectBinding),
		saml.HTTPRedirectBinding,
		saml.HTTPPostBinding,
	)
	if err != nil {
		return nil, fmt.Errorf("samlsp: creating authn request: %w", err)
	}

	// Store the request ID for InResponseTo validation.
	p.requestStore.Store(authReq.ID)

	redirectURL, err := authReq.Redirect(relayState, &p.sp)
	if err != nil {
		return nil, fmt.Errorf("samlsp: building redirect URL: %w", err)
	}

	p.logger("INFO", fmt.Sprintf("SAML AuthnRequest created: id=%s", authReq.ID))
	return redirectURL, nil
}

// ParseACSResponse validates the SAML response from the ACS POST and
// extracts user information. It validates signatures, audience, timing,
// and replay protection.
func (p *Provider) ParseACSResponse(r *http.Request) (*UserInfo, error) {
	if err := r.ParseForm(); err != nil {
		return nil, fmt.Errorf("samlsp: parsing form: %w", err)
	}

	p.metadataMu.RLock()
	defer p.metadataMu.RUnlock()

	assertion, err := p.sp.ParseResponse(r, p.requestStore.possibleRequestIDs())
	if err != nil {
		return nil, fmt.Errorf("samlsp: validating response: %w", err)
	}

	// Extract the InResponseTo to clean up the request store.
	if assertion.Subject != nil && assertion.Subject.SubjectConfirmations != nil {
		for _, sc := range assertion.Subject.SubjectConfirmations {
			if sc.SubjectConfirmationData != nil && sc.SubjectConfirmationData.InResponseTo != "" {
				p.requestStore.Delete(sc.SubjectConfirmationData.InResponseTo)
			}
		}
	}

	// Optionally dump the full decrypted assertion for troubleshooting.
	p.logAssertionIfEnabled(assertion)

	// Build UserInfo from the assertion.
	info := p.extractUserInfo(assertion)

	p.logger("INFO", fmt.Sprintf("SAML assertion validated: subject=%s username=%s groups=%v role=%s",
		info.SAMLSubject, info.Username, info.Groups, info.Role))

	return info, nil
}

// ParseSLORequest validates an inbound LogoutRequest from the IdP and
// returns the username (NameID) of the user being logged out.
func (p *Provider) ParseSLORequest(r *http.Request) (string, error) {
	if err := r.ParseForm(); err != nil {
		return "", fmt.Errorf("samlsp: parsing SLO form: %w", err)
	}

	p.metadataMu.RLock()
	defer p.metadataMu.RUnlock()

	// Parse the LogoutRequest from POST form (base64-encoded XML)
	// or from redirect (base64 + DEFLATE encoded).
	rawReq := r.FormValue("SAMLRequest")
	if rawReq == "" {
		return "", errors.New("samlsp: missing SAMLRequest in SLO")
	}

	logoutReq, err := decodeLogoutRequest(rawReq)
	if err != nil {
		return "", fmt.Errorf("samlsp: parsing logout request: %w", err)
	}

	nameID := logoutReq.NameID
	if nameID == nil || nameID.Value == "" {
		return "", errors.New("samlsp: logout request missing NameID")
	}

	// Build the SAML subject key for this user.
	idpEntityID := ""
	if p.idpMetadata != nil {
		idpEntityID = p.idpMetadata.EntityID
	}
	subject := buildSAMLSubject(idpEntityID, nameID.Value)

	p.logger("INFO", fmt.Sprintf("SAML SLO request received: nameID=%s subject=%s", nameID.Value, subject))
	return subject, nil
}

// IDPEntityID returns the entity ID of the configured IdP.
func (p *Provider) IDPEntityID() string {
	p.metadataMu.RLock()
	defer p.metadataMu.RUnlock()
	if p.idpMetadata != nil {
		return p.idpMetadata.EntityID
	}
	return ""
}

// RefreshMetadata fetches fresh IdP metadata and updates the provider.
// Should be called periodically (e.g. every MetadataRefreshInterval).
// Only the URL source is refreshable; file and pasted-XML sources are static,
// so this is a no-op for them.
func (p *Provider) RefreshMetadata(ctx context.Context) error {
	if p.cfg.IDPMetadataURL == "" {
		return nil
	}

	meta, err := fetchIDPMetadata(p.cfg.IDPMetadataURL)
	if err != nil {
		p.logger("ERROR", fmt.Sprintf("SAML metadata refresh failed: %v", err))
		return err
	}

	p.metadataMu.Lock()
	p.idpMetadata = meta
	p.sp.IDPMetadata = meta
	p.metadataMu.Unlock()

	p.logger("INFO", "SAML IdP metadata refreshed successfully")
	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// logAssertionIfEnabled dumps the full decrypted assertion XML when the
// DebugLogAssertions toggle is on. It logs at WARN because the dumped XML
// contains PII and a replayable credential; the toggle is meant to be enabled
// only briefly while diagnosing an IdP integration. No-op when disabled.
func (p *Provider) logAssertionIfEnabled(assertion *saml.Assertion) {
	if !p.cfg.DebugLogAssertions || assertion == nil {
		return
	}
	xmlBytes, err := xml.MarshalIndent(assertion, "", "  ")
	if err != nil {
		p.logger("ERROR", fmt.Sprintf("SAML assertion debug logging: marshal failed: %v", err))
		return
	}
	p.logger("WARN", fmt.Sprintf("SAML assertion debug logging is ENABLED — the following contains PII "+
		"and a replayable credential; disable saml_debug_log_assertions once finished:\n%s", string(xmlBytes)))
}

// extractUserInfo maps SAML assertion attributes to UserInfo.
func (p *Provider) extractUserInfo(assertion *saml.Assertion) *UserInfo {
	attrs := flattenAttributes(assertion)

	// Log all received attributes at DEBUG level to help troubleshoot mappings.
	if p.logger != nil {
		attrNames := make([]string, 0, len(attrs))
		for k, v := range attrs {
			attrNames = append(attrNames, fmt.Sprintf("%s=%v", k, v))
		}
		p.logger("DEBUG", fmt.Sprintf("SAML assertion attributes: %v", attrNames))
	}

	nameID := ""
	if assertion.Subject != nil && assertion.Subject.NameID != nil {
		nameID = assertion.Subject.NameID.Value
	}

	// Resolve username: configured attr > NameID.
	username := nameID
	if p.cfg.UsernameAttr != "" {
		if v, ok := attrs[p.cfg.UsernameAttr]; ok && len(v) > 0 {
			username = v[0]
		}
	}

	// Resolve email.
	email := ""
	emailAttr := p.cfg.EmailAttr
	if emailAttr == "" {
		emailAttr = "email"
	}
	if v, ok := attrs[emailAttr]; ok && len(v) > 0 {
		email = v[0]
	}

	// Resolve display name.
	displayName := ""
	dnAttr := p.cfg.DisplayNameAttr
	if dnAttr == "" {
		dnAttr = "displayName"
	}
	if v, ok := attrs[dnAttr]; ok && len(v) > 0 {
		displayName = v[0]
	}

	// Resolve groups.
	var groups []string
	groupsAttr := p.cfg.GroupsAttr
	if groupsAttr == "" {
		groupsAttr = "groups"
	}
	if v, ok := attrs[groupsAttr]; ok {
		groups = v
	}

	// Map groups to role (or use direct role attribute).
	role := p.resolveRole(groups, attrs)

	// Build stable identity.
	idpEntityID := ""
	if p.idpMetadata != nil {
		idpEntityID = p.idpMetadata.EntityID
	}

	return &UserInfo{
		SAMLSubject: buildSAMLSubject(idpEntityID, nameID),
		Username:    username,
		Email:       email,
		DisplayName: displayName,
		Groups:      groups,
		Role:        role,
	}
}

// resolveRole maps group memberships to the highest-privilege matching role.
// If RoleAttr is configured and present in attrs, it takes precedence.
func (p *Provider) resolveRole(groups []string, attrs map[string][]string) string {
	validRoles := map[string]int{"viewer": 1, "operator": 2, "admin": 3}

	// Check direct role attribute first.
	if p.cfg.RoleAttr != "" {
		if v, ok := attrs[p.cfg.RoleAttr]; ok && len(v) > 0 {
			role := strings.ToLower(strings.TrimSpace(v[0]))
			if _, valid := validRoles[role]; valid {
				return role
			}
		}
	}

	// Fall back to group-based mapping.
	if len(p.cfg.RoleMapping) == 0 {
		return "viewer"
	}

	best := "viewer"
	bestPriority := 1

	for _, group := range groups {
		if role, ok := p.cfg.RoleMapping[group]; ok {
			if pri, known := validRoles[role]; known && pri > bestPriority {
				best = role
				bestPriority = pri
			}
		}
	}

	return best
}

// buildSAMLSubject creates the stable federated identity key.
func buildSAMLSubject(idpEntityID, nameID string) string {
	return idpEntityID + ":" + nameID
}

// flattenAttributes extracts all attribute values from an assertion into
// a map keyed by attribute name.
func flattenAttributes(assertion *saml.Assertion) map[string][]string {
	attrs := make(map[string][]string)
	for _, stmt := range assertion.AttributeStatements {
		for _, attr := range stmt.Attributes {
			var values []string
			for _, v := range attr.Values {
				values = append(values, v.Value)
			}
			attrs[attr.Name] = values
			// Also index by FriendlyName if present.
			if attr.FriendlyName != "" && attr.FriendlyName != attr.Name {
				attrs[attr.FriendlyName] = values
			}
		}
	}
	return attrs
}

// parseCertAndKey parses PEM-encoded certificate and private key.
func parseCertAndKey(certPEM, keyPEM []byte) (*x509.Certificate, *rsa.PrivateKey, error) {
	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("parsing certificate/key pair: %w", err)
	}

	cert, err := x509.ParseCertificate(tlsCert.Certificate[0])
	if err != nil {
		return nil, nil, fmt.Errorf("parsing x509 certificate: %w", err)
	}

	key, ok := tlsCert.PrivateKey.(*rsa.PrivateKey)
	if !ok {
		return nil, nil, errors.New("private key must be RSA")
	}

	return cert, key, nil
}

// fetchIDPMetadata fetches and parses IdP metadata from the given URL.
func fetchIDPMetadata(metadataURL string) (*saml.EntityDescriptor, error) {
	if !strings.HasPrefix(strings.ToLower(metadataURL), "https://") {
		return nil, fmt.Errorf("metadata URL must be https: %s", metadataURL)
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(metadataURL)
	if err != nil {
		return nil, fmt.Errorf("fetching metadata: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metadata fetch returned status %d", resp.StatusCode)
	}

	// Limit response size to 1MB.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading metadata body: %w", err)
	}

	descriptor, err := samlsp.ParseMetadata(body)
	if err != nil {
		return nil, fmt.Errorf("parsing metadata XML: %w", err)
	}

	return descriptor, nil
}

// loadIDPMetadataFromFile reads and parses IdP metadata from a local file.
func loadIDPMetadataFromFile(path string) (*saml.EntityDescriptor, error) {
	body, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading metadata file %q: %w", path, err)
	}
	if len(body) > 1<<20 {
		return nil, fmt.Errorf("metadata file %q exceeds 1MB size limit", path)
	}

	descriptor, err := samlsp.ParseMetadata(body)
	if err != nil {
		return nil, fmt.Errorf("parsing metadata XML from %q: %w", path, err)
	}

	return descriptor, nil
}

// loadIDPMetadataFromXML parses IdP metadata from raw XML pasted into the
// configuration (the "paste" source). The same 1MB size limit as the file and
// URL loaders applies.
func loadIDPMetadataFromXML(body []byte) (*saml.EntityDescriptor, error) {
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil, errors.New("idp metadata XML is empty")
	}
	if len(body) > 1<<20 {
		return nil, fmt.Errorf("idp metadata XML exceeds 1MB size limit")
	}

	descriptor, err := samlsp.ParseMetadata(body)
	if err != nil {
		return nil, fmt.Errorf("parsing pasted metadata XML: %w", err)
	}

	return descriptor, nil
}

// decodeLogoutRequest decodes a base64-encoded (optionally DEFLATE-compressed)
// SAML LogoutRequest from either a POST form or redirect binding.
func decodeLogoutRequest(encoded string) (*saml.LogoutRequest, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode: %w", err)
	}

	// Try to inflate (redirect binding uses DEFLATE). If inflate fails,
	// assume it's already plain XML (POST binding).
	reader := flate.NewReader(strings.NewReader(string(raw)))
	inflated, inflateErr := io.ReadAll(reader)
	reader.Close()

	xmlData := raw
	if inflateErr == nil && len(inflated) > 0 {
		xmlData = inflated
	}

	var req saml.LogoutRequest
	if err := xml.Unmarshal(xmlData, &req); err != nil {
		return nil, fmt.Errorf("unmarshal LogoutRequest: %w", err)
	}

	return &req, nil
}
