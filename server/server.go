package main

import (
	"flag"
	"fmt"
	"log"
	"net"
	"net/rpc"
	"uk.ac.bris.cs/gameoflife/stubs"
)

type Server struct{

}

func (s *Server) HandleGOLRequest(request stubs.GameOfLifeRequest, response *stubs.GameOfLifeResponse) (err error) {
	result := nextIteration(request.Height,
		request.Width,
		request.World,
		request.StartY,
		request.EndY,
		request.StartX,
		request.EndX)
	response.World = result
	response.Turn = request.Turn
	response.Block = request.Block
	return
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

func nextIteration(height, width int, world [][]uint8, startY, endY, startX, endX int) [][]uint8 {
	fmt.Println(endY - startY)
	fmt.Println(endY)
	fmt.Println(startY)
	newWorld := make([][]uint8, endY-startY)
	for x := startY; x < endY; x++ {
		newWorld[x-startY] = make([]uint8, endX-startX)
	}
	for y := startY; y < endY; y++ {
		for x := startX; x < endX; x++ {
			neighbours := calculateNeighbours(height, width, world, x, y)
			if neighbours < 2 || neighbours > 3 {
				newWorld[y-startY][x-startX] = 0

			} else if neighbours == 3 {
				newWorld[y-startY][x-startX] = 255

			} else {
				newWorld[y-startY][x-startX] = world[y][x]
			}
		}
	}
	return newWorld
}

func main() {
	pAddr := flag.String("port", "8070", "port to listen on")
	flag.Parse()
	listener, err := net.Listen("tcp", ":"+*pAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	defer listener.Close()

	rpc.Register(&Server{})
	rpc.Accept(listener)
}
