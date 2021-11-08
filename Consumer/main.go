package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	pb "SiteSurvey/sitesurveypb"

	"google.golang.org/grpc"
)

func main() {
	wg := &sync.WaitGroup{}
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(wg *sync.WaitGroup) {
			defer wg.Done()
			conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
			if err != nil {
				log.Printf("did not connect: %v\n", err)
			}
			defer conn.Close()
			c := pb.NewSurveyClient(conn)

			ctx, cancel := context.WithTimeout(context.Background(), time.Second*2)
			defer cancel()
			r, err := c.SendSurvey(ctx, &pb.SurveyRequest{})
			if err != nil {
				log.Printf("could not response: %v\n", err)
				return
			}
			for _, body := range r.GetReceive() {
				if body.Error != "" {
					fmt.Printf("%s\n", body.Error)
				}
				fmt.Print(body.Body)
			}
		}(wg)
	}
	wg.Wait()
	log.Printf("\nEXIT\n")
}
