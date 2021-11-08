package main

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"sync"
	"time"

	pb "SiteSurvey/sitesurveypb"

	"github.com/go-redis/redis/v8"
	"google.golang.org/grpc"
	"gopkg.in/yaml.v2"
)

var (
	port     = flag.Int("port", 50051, "The server port")
	fileName = flag.String("fileName", "../data/config.yml", "")
)

type DataRowUrl struct {
	Data string `json:"data"`
	Age  int    `json:"age"`
}

type server struct {
	pb.UnimplementedSurveyServer
	data DataSurvey
	rdb  *redis.Client
	hash string
}

type DataSurvey struct {
	URLs             []string `yaml:"URLs"`
	MinTimeout       int      `yaml:"MinTimeout"`
	MaxTimeout       int      `yaml:"MaxTimeout"`
	NumberOfRequests int      `yaml:"NumberOfRequests"`
}

func (s *server) SendSurvey(ctx context.Context, in *pb.SurveyRequest) (*pb.SurveyReply, error) {
	log.Printf("Receive survey")
	m := make(chan *pb.OneReceive)
	var wg = &sync.WaitGroup{}
	var mu = &sync.Mutex{}
	lenData := len(s.data.URLs)
	rand.Seed(time.Now().UnixNano())
	rnd := func(len int) int {
		return rand.Intn(len)
	}
	body := make([]*pb.OneReceive, 0, s.data.NumberOfRequests)
	for i := 0; i < s.data.NumberOfRequests; i++ {
		rndUrl := s.data.URLs[rnd(lenData)]
		log.Printf("rand url = %v\n", rndUrl)
		val, err := s.rdb.Get(ctx, rndUrl).Result()
		if err != nil {
			wg.Add(1)
			go func(url string, m chan<- *pb.OneReceive, wg *sync.WaitGroup, mu *sync.Mutex, s *server, ctx context.Context) {
				log.Printf("Receive survey url = %v\n", url)
				rndDuration := rand.Intn(s.data.MaxTimeout-s.data.MaxTimeout+1) + s.data.MinTimeout
				flagTimeout := true
				var resp *http.Response
				go func(resp *http.Response, m chan<- *pb.OneReceive, flagTimeout *bool, wg *sync.WaitGroup, mu *sync.Mutex) {
					time.Sleep(time.Second * 2)
					mu.Lock()
					if *flagTimeout == true {
						log.Printf("Done_timeout url = %v\n", url)
						m <- &pb.OneReceive{
							Body:  "",
							Error: "failed to receive in 1s",
						}
						*flagTimeout = false
						mu.Unlock()
						wg.Done()
					}
				}(resp, m, &flagTimeout, wg, mu)
				resp, err := http.Get(url)
				mu.Lock()
				if flagTimeout == true && resp != nil {
					log.Printf("Done url = %v\n", url)
					flagTimeout = false
					mu.Unlock()
					defer wg.Done()
					defer resp.Body.Close()
					if err != nil {
						fmt.Println(err)
						m <- &pb.OneReceive{
							Body:  "",
							Error: err.Error(),
						}
						return
					}
					bodyBytes, err := ioutil.ReadAll(resp.Body)
					if err != nil {
						m <- &pb.OneReceive{
							Body:  "",
							Error: err.Error(),
						}
						return
					}
					body := string(bodyBytes)
					timeSec := time.Second * time.Duration(rndDuration)
					flag, err := s.rdb.SetNX(ctx, url, body, timeSec).Result()
					if err != nil {
						fmt.Printf("set error = %v", err.Error())
					}
					if flag == false {
						val, err := s.rdb.Get(ctx, url).Result()
						if err != nil {
							fmt.Printf("set--> get error = %v", err.Error())
						} else {
							fmt.Printf("get old version = %v", len(val))
							body = val
						}
					}
					log.Printf("url = %v timeSec = %v len_body = %v", url, timeSec, len(body))
					m <- &pb.OneReceive{
						Body:  body,
						Error: "",
					}
				}

			}(rndUrl, m, wg, mu, s, ctx)
		} else {
			body = append(body, &pb.OneReceive{
				Body:  val,
				Error: "",
			})
		}
	}
	go func(m chan *pb.OneReceive, wg *sync.WaitGroup) {
		wg.Wait()
		close(m)
	}(m, wg)

	for rec := range m {
		//log.Printf("rec = %v", rec)
		body = append(body, rec)
	}

	return &pb.SurveyReply{
			Receive: body,
		},
		nil
}

func main() {
	flag.Parse()
	lis, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", *port))
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}
	yamlFile, err := ioutil.ReadFile(*fileName)
	if err != nil {
		log.Fatalf("Error reading YAML file: %s\n", err)
	}

	serv := &server{hash: "qerwerwer"}
	err = yaml.Unmarshal(yamlFile, &serv.data)
	if err != nil {
		log.Fatalf("Error parsing YAML file: %s\n", err)
	}

	serv.rdb = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	if serv.rdb == nil {
		log.Fatalf("failed connect to redis\n")
	} else {
		log.Printf("done connect to redis\n")
	}

	s := grpc.NewServer()
	pb.RegisterSurveyServer(s, serv)
	log.Printf("server listening at %v", lis.Addr())
	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
