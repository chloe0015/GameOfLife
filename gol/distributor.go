package gol

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"
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

var turn int

func sendCellFlipped(events chan<- Event, turn int, cell util.Cell) {
	events <- CellFlipped{turn, cell}
}

func mod(n, m int) int {
	return (n + m) % m
}

func calculateNeighbours(height int, width int, world [][]uint8, i int, j int) int {
	count := 0
	for x := -1; x <= 1; x++ {
		for y := -1; y <= 1; y++ {
			if x != 0 || y != 0 {
				if world[mod(j+x, height)][mod(i+y, width)] == 255 {
					count++
				}
			}
		}
	}
	return count
}

func nextIteration(height, width int, world [][]uint8, c distributorChannels, startY, endY, startX, endX int) [][]uint8 {
	newWorld := make([][]uint8, len(world))
	for x := 0; x < width; x++ {
		newWorld[x] = make([]uint8, height)
	}
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			neighbours := calculateNeighbours(height, width, world, x, y)
			if neighbours < 2 || neighbours > 3 {
				newWorld[y][x] = 0
				if newWorld[y][x] != world[y][x] {
					cell := util.Cell{Y: y, X: x}
					sendCellFlipped(c.events, turn, cell)
				}

			} else if neighbours == 3 {
				newWorld[y][x] = 255
				if newWorld[y][x] != world[y][x] {
					cell := util.Cell{Y: y, X: x}
					sendCellFlipped(c.events, turn, cell)
				}
			} else {
				newWorld[y][x] = world[y][x]
			}
		}
	}
	return newWorld
}

func next(p Params, world [][]uint8, c distributorChannels, startY, endY, startX, endX int, channel chan [][]uint8) {
	b := nextIteration(p.ImageHeight, p.ImageWidth, world, c, startY, endY, startX, endX)
	channel <- b
}

func createChannels(p Params) []chan [][]uint8 {
	channels := make([]chan [][]uint8, p.Threads)
	for i := range channels {
		channels[i] = make(chan [][]uint8)
	}
	return channels
}

func processPgm(channels []chan [][]uint8, p Params) [][]uint8 {
	var finalWorld [][]uint8
	for i := 0; i < p.Threads; i++ {
		b := <-channels[i]
		finalWorld = append(finalWorld, b...)
	}
	return finalWorld
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
	outputString := strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + "x" + strconv.Itoa(p.Turns)
	c.ioFilename <- outputString
	for x := 0; x < p.ImageHeight; x++ {
		for y := 0; y < p.ImageWidth; y++ {
			c.ioOutput <- world[x][y]
		}
	}
	c.events <- ImageOutputComplete{CompletedTurns: turn, Filename: outputString}
}

//TODO ticker
func trackTicker(event chan<- Event, tickerDone chan bool, c distributorChannels, p Params, world [][]uint8) {
	ticker := time.NewTicker(2 * time.Second)
	for {
		select {
		case <-ticker.C:
			c.events <- AliveCellsCount{turn, len(countAliveCells(p, world))}
		case <-tickerDone:
			return
		}
	}
}

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

	// TODO: Execute all turns of the Game of Life.
	// TODO: Report the final state using FinalTurnCompleteEvent.

	turn := 0
	c.ioCommand <- ioInput
	c.ioFilename <- strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth)
	//:= strconv.Itoa(p.ImageHeight) + "x" + strconv.Itoa(p.ImageWidth) + strconv.Itoa(p.Turns)
	mutex := &sync.Mutex{}
	tickerDone := make(chan bool)

	//create world
	world := make([][]uint8, p.ImageHeight)

	for x := 0; x < p.ImageWidth; x++ {
		world[x] = make([]uint8, p.ImageHeight)
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
	go trackTicker(c.events, tickerDone, c, p, world)
	go keyPresses(p, c, world, mutex, turn)
	channels := createChannels(p)
	length := p.ImageHeight / p.ImageWidth
	for x := 0; x <= p.Turns; x++ {
		y := 0
		for y < p.Threads-1 {
			go next(p, world, c, y*length, (y+1)*length, 0, p.ImageWidth, channels[y])
			y++
		}
		go next(p, world, c, y*length, p.ImageHeight, 0, p.ImageWidth, channels[p.Threads-1])
		mutex.Lock()
		world = processPgm(channels, p)
		mutex.Unlock()
		turn++
		c.events <- TurnComplete{turn}
	}

	//for x := 0; x < p.Turns; x++ {
	//	//calcuate next world
	//	world = nextIteration(p.ImageHeight, p.ImageWidth, world, c, 0, p.ImageHeight, 0, p.ImageWidth)
	//	c.events <- TurnComplete{turn}
	//	turn++
	//}
	tickerDone <- true

	c.events <- FinalTurnComplete{turn, countAliveCells(p, world)}

	outputPgm(p, c, world)

	c.ioCommand <- ioCheckIdle // Make sure that the Io has finished any output before exiting.
	<-c.ioIdle
	c.events <- StateChange{turn, Quitting}

	// Close the channel to stop the SDL goroutine gracefully. Removing may cause deadlock.
	close(c.events)
}
