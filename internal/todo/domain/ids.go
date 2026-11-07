package domain

// Distinct id types make swapped arguments, such as Get(ctx, listID, ownerID),
// a compile error.
type (
	// OwnerID identifies the user who owns lists, as issued by the identity context.
	OwnerID int64
	ListID  int64
	ItemID  int64
)
