// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package analysis

import (
	"strings"
	"testing"
)

// enrichOffenses must attach per-message remediation for poly-method cops, so a
// Socket.gethostbyname offence carries Addrinfo guidance (not File.exist?).

func TestEnrichOffenses_PolyCop_PerMessageRemediation(t *testing.T) {
	offenses := []CookstyleOffense{
		{CopName: "Lint/DeprecatedClassMethods", Severity: "warning", Message: "`Socket.gethostbyname` is deprecated in favor of `Addrinfo.getaddrinfo`."},
		{CopName: "Lint/DeprecatedClassMethods", Severity: "warning", Message: "`File.exists?` is deprecated in favor of `File.exist?`."},
	}
	got := enrichOffenses(offenses)
	if len(got) != 2 {
		t.Fatalf("got %d enriched offences, want 2", len(got))
	}

	socket := got[0].Remediation
	if socket == nil {
		t.Fatal("Socket offence has no remediation")
	}
	if socket.RemovedIn != "" {
		t.Errorf("Socket remediation RemovedIn = %q, want empty (deprecation-only)", socket.RemovedIn)
	}
	if !strings.Contains(socket.ReplacementPattern, "Addrinfo") || strings.Contains(socket.ReplacementPattern, "File.exist") {
		t.Errorf("Socket remediation should carry Addrinfo guidance, got %q", socket.ReplacementPattern)
	}

	file := got[1].Remediation
	if file == nil {
		t.Fatal("File.exists? offence has no remediation")
	}
	if file.RemovedIn != "19.0" || !strings.Contains(file.ReplacementPattern, "File.exist?") {
		t.Errorf("File.exists? remediation wrong: RemovedIn=%q pattern=%q", file.RemovedIn, file.ReplacementPattern)
	}
}
