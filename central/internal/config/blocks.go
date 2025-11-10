package config

// DefaultBlocks returns the default block definitions shared with chunk
// servers for world generation.
func DefaultBlocks() []BlockDefinition {
	return []BlockDefinition{
		{
			ID:    "dirt",
			Color: "#8B5A2B",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 32, VeinSizeMax: 96},
		},
		{
			ID:    "sand",
			Color: "#C2B280",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 24, VeinSizeMax: 80},
		},
		{
			ID:    "slate",
			Color: "#2F4F4F",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 16, VeinSizeMax: 48},
		},
		{
			ID:    "sandstone",
			Color: "#D2B48C",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 20, VeinSizeMax: 64},
		},
		{
			ID:    "obsidian",
			Color: "#341A34",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 8, VeinSizeMax: 20},
		},
		{
			ID:    "shale",
			Color: "#4B3F32",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 16, VeinSizeMax: 40},
		},
		{
			ID:    "cobblestone",
			Color: "#8A8A8A",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 24, VeinSizeMax: 70},
		},
		{
			ID:    "coal",
			Color: "#2B2B2B",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 6, VeinSizeMax: 18},
		},
		{
			ID:    "oil_soaked_rock",
			Color: "#3B2F2F",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 4, VeinSizeMax: 12},
		},
		{
			ID:    "iron",
			Color: "#B7410E",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 4, VeinSizeMax: 12},
		},
		{
			ID:    "copper",
			Color: "#B87333",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 4, VeinSizeMax: 14},
		},
		{
			ID:    "gold",
			Color: "#FFD700",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 3, VeinSizeMax: 8},
		},
		{
			ID:    "silver",
			Color: "#C0C0C0",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 3, VeinSizeMax: 8},
		},
		{
			ID:    "uranium",
			Color: "#6B8E23",
			Spawn: BlockSpawnConfig{Type: "vein", VeinSizeMin: 2, VeinSizeMax: 5},
		},
		{
			ID:    "unobtainium",
			Color: "#7F00FF",
			Spawn: BlockSpawnConfig{Type: "solo"},
		},
	}
}
