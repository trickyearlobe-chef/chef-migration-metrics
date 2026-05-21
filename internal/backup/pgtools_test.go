// Copyright 2025 Chef Migration Metrics Authors
// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"testing"
)

func TestParseConnString(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		want    ConnParams
		wantErr bool
	}{
		{
			name: "full URL with sslmode",
			url:  "postgres://myuser:mypass@db.example.com:5433/mydb?sslmode=require",
			want: ConnParams{
				Host:     "db.example.com",
				Port:     "5433",
				User:     "myuser",
				Password: "mypass",
				DBName:   "mydb",
				SSLMode:  "require",
			},
		},
		{
			name: "default port and sslmode",
			url:  "postgres://user:pass@localhost/testdb",
			want: ConnParams{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "pass",
				DBName:   "testdb",
				SSLMode:  "prefer",
			},
		},
		{
			name: "sslmode disable",
			url:  "postgres://chef_migration_metrics:chef_migration_metrics_dev@localhost:5432/chef_migration_metrics?sslmode=disable",
			want: ConnParams{
				Host:     "localhost",
				Port:     "5432",
				User:     "chef_migration_metrics",
				Password: "chef_migration_metrics_dev",
				DBName:   "chef_migration_metrics",
				SSLMode:  "disable",
			},
		},
		{
			name: "no password",
			url:  "postgres://user@localhost/db?sslmode=disable",
			want: ConnParams{
				Host:     "localhost",
				Port:     "5432",
				User:     "user",
				Password: "",
				DBName:   "db",
				SSLMode:  "disable",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseConnString(tt.url)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseConnString() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseConnString() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestConnParams_Env(t *testing.T) {
	conn := ConnParams{
		Host:     "localhost",
		Port:     "5432",
		User:     "myuser",
		Password: "secret",
		DBName:   "mydb",
		SSLMode:  "disable",
	}

	env := conn.Env()
	expected := map[string]string{
		"PGHOST":     "localhost",
		"PGPORT":     "5432",
		"PGUSER":     "myuser",
		"PGDATABASE": "mydb",
		"PGSSLMODE":  "disable",
		"PGPASSWORD": "secret",
	}

	envMap := make(map[string]string)
	for _, e := range env {
		parts := splitFirst(e, "=")
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	for k, want := range expected {
		if got := envMap[k]; got != want {
			t.Errorf("env %s = %q, want %q", k, got, want)
		}
	}
}

func TestConnParams_Env_NoPassword(t *testing.T) {
	conn := ConnParams{
		Host:    "localhost",
		Port:    "5432",
		User:    "myuser",
		DBName:  "mydb",
		SSLMode: "disable",
	}

	env := conn.Env()
	for _, e := range env {
		parts := splitFirst(e, "=")
		if len(parts) == 2 && parts[0] == "PGPASSWORD" {
			t.Error("PGPASSWORD should not be set when password is empty")
		}
	}
}

func splitFirst(s, sep string) []string {
	idx := len(sep)
	for i := 0; i <= len(s)-len(sep); i++ {
		if s[i:i+idx] == sep {
			return []string{s[:i], s[i+idx:]}
		}
	}
	return []string{s}
}
