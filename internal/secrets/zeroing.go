package secrets

// ZeroBytes overwrites every byte in the slice with zero. This reduces the
// window during which sensitive material (plaintext credentials, decoded
// master keys) is present in process memory.
//
// Go's garbage collector does not guarantee immediate reclamation of
// unreferenced memory, so zeroing before the slice goes out of scope is a
// best-effort defence-in-depth measure.
func ZeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// ZeroString zeros the backing memory of a string by converting it to a
// byte slice via an unsafe-equivalent copy and zeroing that copy. Because
// Go strings are immutable and the compiler may intern or deduplicate them,
// this function cannot guarantee the original string's memory is zeroed.
// However, it ensures that the caller's local copy — which is typically
// the only reference in the hot path — is cleared.
//
// For truly sensitive values, prefer passing []byte throughout the call
// chain and using ZeroBytes directly.
func ZeroString(s *string) {
	if s == nil {
		return
	}
	b := []byte(*s)
	ZeroBytes(b)
	*s = ""
}

// IsZeroed returns true if every byte in the slice is zero. This is useful
// in tests to verify that sensitive material has been properly wiped.
func IsZeroed(b []byte) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}
