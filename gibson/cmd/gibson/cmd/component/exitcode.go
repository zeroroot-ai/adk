// SPDX-License-Identifier: Apache-2.0
// Copyright 2026 Zero Root AI

package component

import "fmt"

// ExitCodeError carries a specific process exit code through cobra's error
// return path so that main() can forward it without calling os.Exit deep in
// a handler. Returned by doRun and runValidate instead of os.Exit.
type ExitCodeError struct{ Code int }

func (e ExitCodeError) Error() string { return fmt.Sprintf("exit code %d", e.Code) }
