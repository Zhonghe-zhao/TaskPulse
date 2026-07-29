package domain

type ClaimKind string

const (
	ClaimInitial  ClaimKind = "initial"
	ClaimRetry    ClaimKind = "retry"
	ClaimRecovery ClaimKind = "recovery"
)
