package main

// seedExtraTypes lists types that are reflection-exposed at runtime but not
// directly registered via SetField — for example, types that appear only as
// the return type of a method on an already-seeded type. The closure walker
// reaches most of these automatically; this list is the safety net for
// anything missed.
func seedExtraTypes() []string {
	return []string{
		// Action layer.
		"BufPane",
		"InfoPane",
		"Tab",
		"TabList",
		// Buffer layer.
		"Buffer",
		"Cursor",
		"Loc",
		"Message",
		// Display layer.
		"BWindow",
		"SLoc",
		"VLoc",
		"View",
		// Info layer.
		"InfoBuf",
		// Shell layer.
		"Job",
	}
}
