package secrets

import (
	"testing"
)

// ---------------------------------------------------------------------------
// ZeroBytes tests
// ---------------------------------------------------------------------------

func TestZeroBytes_NonEmpty(t *testing.T) {
	b := []byte{1, 2, 3, 4, 5, 100, 200, 255}
	ZeroBytes(b)

	for i, v := range b {
		if v != 0 {
			t.Fatalf("byte %d is %d, want 0", i, v)
		}
	}
}

func TestZeroBytes_Empty(t *testing.T) {
	b := []byte{}
	ZeroBytes(b) // should not panic
	if len(b) != 0 {
		t.Fatal("empty slice should remain empty")
	}
}

func TestZeroBytes_Nil(t *testing.T) {
	var b []byte
	ZeroBytes(b) // should not panic
}

func TestZeroBytes_SingleByte(t *testing.T) {
	b := []byte{42}
	ZeroBytes(b)
	if b[0] != 0 {
		t.Fatalf("byte is %d, want 0", b[0])
	}
}

func TestZeroBytes_AllZerosAlready(t *testing.T) {
	b := make([]byte, 16)
	ZeroBytes(b) // should not panic or change anything
	if !IsZeroed(b) {
		t.Fatal("slice should still be all zeros")
	}
}

func TestZeroBytes_LargeSlice(t *testing.T) {
	b := make([]byte, 1024*1024) // 1 MB
	for i := range b {
		b[i] = byte(i % 256)
	}

	ZeroBytes(b)

	if !IsZeroed(b) {
		t.Fatal("large slice was not fully zeroed")
	}
}

func TestZeroBytes_PreservesLength(t *testing.T) {
	b := make([]byte, 37)
	for i := range b {
		b[i] = byte(i + 1)
	}
	ZeroBytes(b)

	if len(b) != 37 {
		t.Fatalf("length changed: got %d, want 37", len(b))
	}
	if cap(b) < 37 {
		t.Fatalf("capacity shrunk below original length")
	}
}

func TestZeroBytes_SubSlice(t *testing.T) {
	original := []byte{10, 20, 30, 40, 50, 60, 70, 80}
	sub := original[2:6] // {30, 40, 50, 60}

	ZeroBytes(sub)

	// The sub-slice should be zeroed.
	for i, v := range sub {
		if v != 0 {
			t.Fatalf("sub[%d] = %d, want 0", i, v)
		}
	}

	// The underlying array should be zeroed in the sub-slice range.
	if original[2] != 0 || original[3] != 0 || original[4] != 0 || original[5] != 0 {
		t.Fatal("underlying array was not zeroed in sub-slice range")
	}

	// Elements outside the sub-slice should be untouched.
	if original[0] != 10 || original[1] != 20 || original[6] != 70 || original[7] != 80 {
		t.Fatal("elements outside sub-slice were modified")
	}
}

// ---------------------------------------------------------------------------
// ZeroString tests
// ---------------------------------------------------------------------------

func TestZeroString_NonEmpty(t *testing.T) {
	s := "sensitive password"
	ZeroString(&s)

	if s != "" {
		t.Fatalf("string should be empty after ZeroString, got %q", s)
	}
}

func TestZeroString_Empty(t *testing.T) {
	s := ""
	ZeroString(&s) // should not panic
	if s != "" {
		t.Fatalf("string should remain empty, got %q", s)
	}
}

func TestZeroString_Nil(t *testing.T) {
	ZeroString(nil) // should not panic
}

func TestZeroString_SetsToEmptyString(t *testing.T) {
	s := "my-secret-key-value"
	ZeroString(&s)

	if len(s) != 0 {
		t.Fatalf("string length should be 0, got %d", len(s))
	}
}

func TestZeroString_MultipleCallsSafe(t *testing.T) {
	s := "secret"
	ZeroString(&s)
	ZeroString(&s) // second call on already-empty string
	if s != "" {
		t.Fatalf("string should be empty after double ZeroString, got %q", s)
	}
}

func TestZeroString_LongString(t *testing.T) {
	// Build a long string to exercise the zeroing on larger allocations.
	buf := make([]byte, 4096)
	for i := range buf {
		buf[i] = byte('A' + (i % 26))
	}
	s := string(buf)

	ZeroString(&s)

	if s != "" {
		t.Fatalf("long string should be empty, got length %d", len(s))
	}
}

// ---------------------------------------------------------------------------
// IsZeroed tests
// ---------------------------------------------------------------------------

func TestIsZeroed_AllZeros(t *testing.T) {
	b := make([]byte, 64)
	if !IsZeroed(b) {
		t.Fatal("all-zero slice should return true")
	}
}

func TestIsZeroed_NonZero(t *testing.T) {
	b := make([]byte, 64)
	b[32] = 1
	if IsZeroed(b) {
		t.Fatal("slice with non-zero byte should return false")
	}
}

func TestIsZeroed_FirstByteNonZero(t *testing.T) {
	b := make([]byte, 8)
	b[0] = 0xFF
	if IsZeroed(b) {
		t.Fatal("slice with non-zero first byte should return false")
	}
}

func TestIsZeroed_LastByteNonZero(t *testing.T) {
	b := make([]byte, 8)
	b[7] = 0x01
	if IsZeroed(b) {
		t.Fatal("slice with non-zero last byte should return false")
	}
}

func TestIsZeroed_Empty(t *testing.T) {
	if !IsZeroed([]byte{}) {
		t.Fatal("empty slice should be considered zeroed")
	}
}

func TestIsZeroed_Nil(t *testing.T) {
	if !IsZeroed(nil) {
		t.Fatal("nil slice should be considered zeroed")
	}
}

func TestIsZeroed_SingleZeroByte(t *testing.T) {
	if !IsZeroed([]byte{0}) {
		t.Fatal("single zero byte should be considered zeroed")
	}
}

func TestIsZeroed_SingleNonZeroByte(t *testing.T) {
	if IsZeroed([]byte{42}) {
		t.Fatal("single non-zero byte should not be considered zeroed")
	}
}

// ---------------------------------------------------------------------------
// Integration: ZeroBytes + IsZeroed
// ---------------------------------------------------------------------------

func TestZeroBytes_ThenIsZeroed(t *testing.T) {
	b := []byte("this is secret material that must be wiped")
	if IsZeroed(b) {
		t.Fatal("non-zero data should not be reported as zeroed before wiping")
	}

	ZeroBytes(b)

	if !IsZeroed(b) {
		t.Fatal("data should be reported as zeroed after ZeroBytes")
	}
}

func TestZeroBytes_KeyMaterial(t *testing.T) {
	// Simulate wiping a 32-byte AES key.
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i + 1)
	}

	if IsZeroed(key) {
		t.Fatal("key material should not be zeroed before wipe")
	}

	ZeroBytes(key)

	if !IsZeroed(key) {
		t.Fatal("key material should be zeroed after wipe")
	}
}

func TestZeroBytes_PEMKeySimulation(t *testing.T) {
	// Simulate wiping a PEM-encoded private key.
	pem := []byte(`-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA0Z3VS5JJcds3xfn/yGasdHSn0l3GizfYqH5JM5EaKG7hCdx+
p6b8NcJyuNERh3Lq1FcWVdB8fKOz7AYPhdea1Kw1Q4o3DUI6JsyYE+L06rVN+bAy
-----END RSA PRIVATE KEY-----`)

	if IsZeroed(pem) {
		t.Fatal("PEM data should not be zeroed before wipe")
	}

	ZeroBytes(pem)

	if !IsZeroed(pem) {
		t.Fatal("PEM data should be zeroed after wipe")
	}
	if len(pem) == 0 {
		t.Fatal("ZeroBytes should not change the slice length")
	}
}
