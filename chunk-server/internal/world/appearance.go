package world

// BlockAppearance captures visual styling for a block material.
type BlockAppearance struct {
	Material string
	Color    string
	Texture  string
}

const (
	MaterialGrass = "grass"
	MaterialDirt  = "dirt"
)

// DefaultAppearances enumerates the built-in block visuals.
var DefaultAppearances = map[string]BlockAppearance{
	MaterialGrass: {
		Material: MaterialGrass,
		Color:    "#5d9b3d",
		Texture:  "assets/textures/grass.png",
	},
	MaterialDirt: {
		Material: MaterialDirt,
		Color:    "#8b5a2b",
		Texture:  "assets/textures/dirt.png",
	},
}

// ApplyAppearance copies the known appearance settings for the provided material
// onto the block. If the material is unknown the block retains the provided name
// and leaves color/texture untouched, allowing callers to define their own.
func ApplyAppearance(block *Block, material string) {
	if block == nil {
		return
	}
	block.Material = material
	if preset, ok := DefaultAppearances[material]; ok {
		block.Color = preset.Color
		block.Texture = preset.Texture
	}
}
