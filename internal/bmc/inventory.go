package bmc

// Inventory is the hardware inventory of one server.
type Inventory struct {
	System        SystemInfo    `json:"system"`
	Manager       ManagerInfo   `json:"manager"`
	Processors    []Processor   `json:"processors"`
	Memory        []MemoryDIMM  `json:"memory"`
	Drives        []Drive       `json:"drives"`
	NICs          []NIC         `json:"nics"`
	PowerSupplies []PowerSupply `json:"powerSupplies"`
	Firmware      []Firmware    `json:"firmware"`
	FRU           []FRUInfo     `json:"fru"`
	Source        Protocol      `json:"source"`
	CollectedAt   int64         `json:"collectedAt"`
	Warnings      []string      `json:"warnings"`
}

type SystemInfo struct {
	Manufacturer   string  `json:"manufacturer"`
	Model          string  `json:"model"`
	Serial         string  `json:"serial"`
	SKU            string  `json:"sku"`
	PartNumber     string  `json:"partNumber"`
	UUID           string  `json:"uuid"`
	HostName       string  `json:"hostName"`
	BIOSVersion    string  `json:"biosVersion"`
	PowerState     string  `json:"powerState"`
	Health         string  `json:"health"`
	ProcessorModel string  `json:"processorModel"`
	ProcessorCount int     `json:"processorCount"`
	CoreCount      int     `json:"coreCount"`
	ThreadCount    int     `json:"threadCount"`
	MemoryGiB      float64 `json:"memoryGiB"`
}

type ManagerInfo struct {
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmwareVersion"`
	MACAddress      string `json:"macAddress"`
	IPv4            string `json:"ipv4"`
}

type Processor struct {
	Socket       string `json:"socket"`
	Model        string `json:"model"`
	Manufacturer string `json:"manufacturer"`
	Cores        int    `json:"cores"`
	Threads      int    `json:"threads"`
	MaxSpeedMHz  int    `json:"maxSpeedMHz"`
	Health       string `json:"health"`
}

type MemoryDIMM struct {
	Slot         string `json:"slot"`
	CapacityMiB  int    `json:"capacityMiB"`
	Type         string `json:"type"`
	SpeedMHz     int    `json:"speedMHz"`
	Manufacturer string `json:"manufacturer"`
	PartNumber   string `json:"partNumber"`
	Serial       string `json:"serial"`
	Health       string `json:"health"`
}

type Drive struct {
	Name          string `json:"name"`
	Model         string `json:"model"`
	Serial        string `json:"serial"`
	CapacityBytes int64  `json:"capacityBytes"`
	MediaType     string `json:"mediaType"`
	Protocol      string `json:"protocol"`
	Firmware      string `json:"firmware"`
	Health        string `json:"health"`
}

type NIC struct {
	Name       string   `json:"name"`
	MACAddress string   `json:"macAddress"`
	SpeedMbps  int      `json:"speedMbps"`
	LinkStatus string   `json:"linkStatus"`
	IPv4       []string `json:"ipv4"`
	Health     string   `json:"health"`
}

type PowerSupply struct {
	Name          string  `json:"name"`
	Model         string  `json:"model"`
	Serial        string  `json:"serial"`
	CapacityWatts float64 `json:"capacityWatts"`
	Firmware      string  `json:"firmware"`
	Health        string  `json:"health"`
}

type Firmware struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	Updateable bool   `json:"updateable"`
}

// FRUInfo is the IPMI FRU data of one FRU device.
type FRUInfo struct {
	Device         string `json:"device"`
	ChassisPart    string `json:"chassisPart"`
	ChassisSerial  string `json:"chassisSerial"`
	BoardMfg       string `json:"boardMfg"`
	BoardProduct   string `json:"boardProduct"`
	BoardSerial    string `json:"boardSerial"`
	BoardPart      string `json:"boardPart"`
	ProductMfg     string `json:"productMfg"`
	ProductName    string `json:"productName"`
	ProductPart    string `json:"productPart"`
	ProductVersion string `json:"productVersion"`
	ProductSerial  string `json:"productSerial"`
}
