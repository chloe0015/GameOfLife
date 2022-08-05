package gol

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"uk.ac.bris.cs/gameoflife/util"
)

type distributorChannels struct {
	events     chan<- Event
	ioCommand  chan<- ioCommand
	ioIdle     <-chan bool
	ioFilename chan<- string
	ioOutput   chan<- uint8
	ioInput    <-chan uint8
	keyPresses <-chan rune
}

func mod(n, m int) int {
	return (n + m) % m
}

func calculateNeighbours(height int, width int, world [][]uint8, i int, j int) int {
	count := 0
	for x := 0; x <= 1; x++ {
		for y := 0; y <= 1; y++ {
			if x != 0 || y != 0 {
				if world[mod(j+x, height)][mod(i+y, width)] == 255 {
					count++
				}
			}
		}
	}
	return count
}

func nextIteration(height, width int, world [][]uint8, startY, finishY, startX, finishX int, next chan [][]uint8) {
	newWorld := make([][]uint8, len(world))
	for x := 0; x < width; x++ {
		newWorld[x] = make([]uint8, height)
	}
	for y := startY; y < finishY; y++ {
		for x := startX; x < finishX; x++ {
			neighbours := calculateNeighbours(height, width, world, x, y)
			if neighbours < 2 || neighbours > 3 {
				newWorld[y][x] = 0
			} else if neighbours == 3 {
				newWorld[y][x] = 255
			} else {
				newWorld[y][x] = world[y][x]
			}
		}
	}
	next <- newWorld
}

func countAliveCells(p Params, world [][]uint8) []util.Cell {
	var cells []util.Cell
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			if world[x][y] == 255 {
				cells = append(cells, util.Cell{X: y, Y: x})
			}
		}
	}
	return cells
}

func outputPgm(p Params, c distributorChannels, world [][]uint8) {
	c.ioCommand <- ioOutput
	outputsString := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.Turns)
	c.ioFilename <- outputsString
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			c.ioOutput <- world[x][y]
		}
	}
}

//TODO ticker
//func trackTicker(ticker *time.Ticker, sum *int){
//	for{
//		select{
//		case <-ticker.C:
//			mutex.Lock()
//			c.events <- AliveCellsCount{turn, *sum}
//			mutex.Unlock()
//		}
//	}
//}

//TODO:keypress os.Exit是啥呀
func keyPresses(p Params, c distributorChannels, world [][]uint8, mutex *sync.Mutex, turn int) {
	switch <-c.keyPresses {
	case 's':
		outputPgm(p, c, world)
	case 'q':
		outputPgm(p, c, world)
		os.Exit(1)
		turn = p.Turns
	case 'p':
		fmt.Println(turn)
		mutex.Lock()
		if <-c.keyPresses == 'p' {
			fmt.Println("Continuing")
			mutex.Unlock()
		}
	}
}

// distributor divides the work between workers and interacts with other goroutines.
func distributor(p Params, c distributorChannels) {

	// TODO: Create a 2D slice to store the world.
	// TODO: Execute all turns of the Game of Life.
	// TODO: Report the final state using FinalTurnCompleteEvent.

	turn := 0
	c.ioCommand <- ioInput
	string := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)
	c.ioFilename <- string

	//create world
	world := make([][]uint8, p.ImageHeight)
	for x := 0; x < p.ImageWidth; x++ {
		world[x] = make([]uint8, p.ImageWidth)
	}

	//make initial turn
	for x := 0; x < p.ImageWidth; x++ {
		for y := 0; y < p.ImageHeight; y++ {
			world[x][y] = <-c.ioInput
			if world[x][y] == 255 {
				c.events <- CellFlipped{turn, util.Cell{Y: x, X: y}}
			}
		}
	}

	outputPgm(p, c, world)

	c.events <- FinalTurnComplete{turn, countAliveCells(p, world)}
	c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
