package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

const (
	baseURL     = "http://localhost:8080"
	totalUsers  = 1000
	concurrency = 100
)

type LoginResp struct {
	Token string `json:"token"`
}

func login(userID int) (string, error) {
	body := fmt.Sprintf(`{"user_id":"%d"}`, userID)

	resp, err := http.Post(
		baseURL+"/login",
		"application/json",
		bytes.NewBufferString(body),
	)

	if err != nil {
		return "", err
	}

	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var loginResp LoginResp
	err = json.Unmarshal(data, &loginResp)
	if err != nil {
		return "", err
	}

	if loginResp.Token == "" {
		return "", fmt.Errorf("empty token, response=%s", string(data))
	}

	return loginResp.Token, nil
}

func seckill(token string) (int, error) {
	req, err := http.NewRequest(
		http.MethodGet,
		baseURL+"/seckill",
		nil,
	)

	if err != nil {
		return 0, err
	}

	req.Header.Set("Authorization", "Bearer "+token)

	client := &http.Client{
		Timeout: 5 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}

	defer resp.Body.Close()

	_, _ = io.ReadAll(resp.Body)

	return resp.StatusCode, nil
}

func main() {
	start := time.Now()

	var success int64
	var failed int64

	jobs := make(chan int, totalUsers)

	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)

		go func() {
			defer wg.Done()

			for userID := range jobs {
				token, err := login(userID)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}

				statusCode, err := seckill(token)
				if err != nil {
					atomic.AddInt64(&failed, 1)
					continue
				}

				if statusCode == http.StatusOK {
					atomic.AddInt64(&success, 1)
				} else {
					atomic.AddInt64(&failed, 1)
				}
			}
		}()
	}

	for i := 1; i <= totalUsers; i++ {
		jobs <- i
	}

	close(jobs)

	wg.Wait()

	cost := time.Since(start)

	fmt.Println("========== Pressure Test Result ==========")
	fmt.Println("Total users:", totalUsers)
	fmt.Println("Concurrency:", concurrency)
	fmt.Println("Success:", success)
	fmt.Println("Failed:", failed)
	fmt.Println("Total time:", cost)
	fmt.Printf("Approx QPS: %.2f\n", float64(totalUsers)/cost.Seconds())
}
