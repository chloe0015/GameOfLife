package stubs

type GameOfLifeRequest struct {
	World  [][]uint8
	StartX int
	EndX   int
	StartY int
	EndY   int
	Turn   int
	Block  int //block排序，按顺序拼图
}

type GameOfLifeResponse struct {
	World [][]uint8
	Turn  int
	Block int
}
