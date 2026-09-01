package server

// Common server return status codes.
const (
	// C_OK indicates successful execution.
	C_OK = 0
	// C_ERR indicates general error during execution.
	C_ERR = -1
	// C_RETRY indicates the operation should be retried.
	C_RETRY = -2
)

