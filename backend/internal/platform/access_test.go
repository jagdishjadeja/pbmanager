package platform

import "testing"

func TestResourceAndFileAccountValidation(t *testing.T) {
	for _, value := range []int{0, 127, 129, 32769, -64} {
		if _, err := MemoryLimit(value); err == nil {
			t.Fatalf("invalid RAM accepted: %d", value)
		}
	}
	for _, value := range []int{128, 256, 512, 1024, 32768} {
		if bytes, err := MemoryLimit(value); err != nil || bytes != int64(value)*MiB {
			t.Fatal("valid RAM rejected")
		}
	}
	for _, name := range []string{"root", "admin", "abc;id", "../abc", "abc-123", "123abc"} {
		if ValidateSFTPUsername(name) == nil {
			t.Fatalf("invalid username accepted: %s", name)
		}
	}
	if ValidateSFTPUsername("abc123") != nil {
		t.Fatal("valid username rejected")
	}
	for _, password := range []string{"short", "long-but:unsafe-password", "password-with\nnewline"} {
		if ValidateSFTPPassword(password) == nil {
			t.Fatal("unsafe password accepted")
		}
	}
}
