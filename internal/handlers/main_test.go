package handlers

import (
	"fmt"
	"os"
	"testing"

	"github.com/routatic/proxy/internal/testutil/tokenfixture"
)

func TestMain(m *testing.M) {
	if err := tokenfixture.Prime(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(m.Run())
}
