package listEntryNonNullablePatcher_test

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRewriter(t *testing.T) {
	path, err := os.Getwd()
	require.NoError(t, err)
	testPackagePath := fmt.Sprintf("%s/testdata/src/a", path)
	testFilePath := fmt.Sprintf("%s/a.go", testPackagePath)
	goldenFilePath := fmt.Sprintf("%s/a.golden", testPackagePath)

	defer func() {
		// Resume the original file
		originalFilePath := fmt.Sprintf("%s/a.original", testPackagePath)
		originalFile, err := ioutil.ReadFile(originalFilePath)
		require.NoError(t, err)
		require.NoError(t, ioutil.WriteFile(testFilePath, originalFile, 0644))
	}()

	cmd := exec.Command("go", "run", fmt.Sprintf("%s/../main.go", path), "-fix", testPackagePath)
	cmd.Run()

	testFile, err := ioutil.ReadFile(testFilePath)
	require.NoError(t, err)

	goldenFile, err := ioutil.ReadFile(goldenFilePath)
	require.NoError(t, err)

	require.Equal(t, goldenFile, testFile)
}
