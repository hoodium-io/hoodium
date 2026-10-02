package types

const (
	RUNETokenPrecompileAddress       = "0x19be000000000000000000000000000000000000"
	RUNETokenPrecompileLatestVersion = 1
)

const (
	HOODITokenPrecompileAddress       = "0x19be000000000000000000000000000000000001"
	HOODITokenPrecompileLatestVersion = 1
)

const (
	PriceOraclePrecompileAddress       = "0x19be000000000000000000000000000000000002"
	PriceOraclePrecompileLatestVersion = 1
)

const (
	StakingPrecompileAddress       = "0x19be000000000000000000000000000000000003"
	StakingPrecompileLatestVersion = 1
)

// Start the TestBed precompile address with the prefix 0x19be1 in order
// to not conflict with any production precompile.
const (
	TestBedPrecompileAddress       = "0x19BE100000000000000000000000000000000000"
	TestBedPrecompileLatestVersion = 1
)

// DefaultPrecompilesVersions is a list of default precompiles and their versions.
// Order of precompiles is important. If changed on a live network, it will break
// the consensus.
var DefaultPrecompilesVersions = []*PrecompileVersionInfo{
	{RUNETokenPrecompileAddress, RUNETokenPrecompileLatestVersion},
	{HOODITokenPrecompileAddress, HOODITokenPrecompileLatestVersion},
	{PriceOraclePrecompileAddress, PriceOraclePrecompileLatestVersion},
	{StakingPrecompileAddress, StakingPrecompileLatestVersion},
	{TestBedPrecompileAddress, TestBedPrecompileLatestVersion},
}
