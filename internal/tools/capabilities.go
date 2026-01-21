package tools

type Capability string

const (
	CapRead    Capability = "read"    // Reading files or memory
	CapWrite   Capability = "write"   // Modifying the filesystem
	CapExecute Capability = "execute" // Running shell commands
	CapNetwork Capability = "network" // Accessing the internet/network
	CapImport  Capability = "import"  // Importing/Requiring external modules
)

var PolicyMap = map[Capability]string{
	CapRead:    "always_allowed",
	CapWrite:   "AllowFileWrite",
	CapExecute: "AllowShell",
	CapNetwork: "AllowNetwork",
}

// IsReadOnly returns true if the capability set implies no side effects
func IsReadOnly(caps []Capability) bool {
	for _, c := range caps {
		if c != CapRead {
			return false
		}
	}
	return true
}

// HasCapability checks if a specific capability exists in the set
func HasCapability(caps []Capability, target Capability) bool {
	for _, c := range caps {
		if c == target {
			return true
		}
	}
	return false
}
