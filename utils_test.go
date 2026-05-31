package tmhi

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_sha256Hash(t *testing.T) {
	result := sha256Hash("admin", "password")
	assert.Equal(t, "ux+w+s92nXMGACVBFqXMzkpsDxdWeI/aFC8GPNGAKqM=", result)
}

func Test_base64urlEscape(t *testing.T) {
	out := base64urlEscape("efbgOrynhgggULfrXxDu9FveT+q2fXegZs6rXIbiky4=")
	assert.Equal(t, "efbgOrynhgggULfrXxDu9FveT-q2fXegZs6rXIbiky4.", out)
}

func Test_sha256URL(t *testing.T) {
	out := sha256URL("admin", "efbgOrynhgggULfrXxDu9FveT+q2fXegZs6rXIbiky4=")
	assert.Equal(t, "xrNe9hWWlAiL14wfvJxcXOBmMKLBOPIXX1nESQpvaOk.", out)
}

func Test_random16bytes(t *testing.T) {
	out1 := random16bytes()
	assert.NotEmpty(t, out1)

	out2 := random16bytes()
	assert.NotEmpty(t, out2)
	assert.NotEqual(t, out1, out2)
}
