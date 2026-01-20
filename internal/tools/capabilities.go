package tools

type Capability string

const (
	CapRead    Capability = "read"    // Reading files or memory
	CapWrite   Capability = "write"   // Modifying the filesystem
	CapExecute Capability = "execute" // Running shell commands
	CapNetwork Capability = "network" // Accessing the internet/network
	CapImport  Capability = "import"  // Importing/Requiring external modules
)

// PolicyMap maps high-level policy flags to required capabilities
var PolicyMap = map[Capability]string{
	CapRead:    "always_allowed", // Usually implicitly allowed
	CapWrite:   "AllowFileWrite",
	CapExecute: "AllowShell",
	CapNetwork: "AllowNetwork", // Future expansion
}
