/*
 * A smart Hub for holding server stat
 * http://www.likexian.com/
 *
 * Copyright 2015, Li Kexian
 * Released under the Apache License, Version 2.0
 *
 */

package main

import (
    "bytes"
    "crypto/tls"
    "crypto/x509"
    "fmt"
    "io/ioutil"
    "net/http"
    "os"
    "strings"
    "time"

    "github.com/likexian/host-stat-go"
    "github.com/likexian/simplejson-go"
)

const (
    CONFIG_FILE = "./client.json"
    PEM_FILE    = "./cert.pem"
)

type Config struct {
    Id     string `json:"id"`
    Name   string `json:"name"`
    Server string `json:"server"`
    Key    string `json:"key"`
}

type Stat struct {
    Id        string  `json:"id"`
    TimeStamp int64   `json:"time_stamp"`
    HostName  string  `json:"host_name"`
    OSRelease string  `json:"os_release"`
    CPUName   string  `json:"cpu_name"`
    CPUCore   uint64  `json:"cpu_core"`
    Uptime    uint64  `json:"uptime"`
    Load      string  `json:"load"`
    CPURate   float64 `json:"cpu_rate"`
    MemRate   float64 `json:"mem_rate"`
    SwapRate  float64 `json:"swap_rate"`
    DiskRate  float64 `json:"disk_rate"`
    DiskWarn  string  `json:"disk_warn"`
    DiskRead  uint64  `json:"disk_read"`
    DiskWrite uint64  `json:"disk_write"`
    NetRead   uint64  `json:"net_read"`
    NetWrite  uint64  `json:"net_write"`
}

func main() {
    time_stamp := time.Now().Unix()
    if !FileExists(CONFIG_FILE) {
        SettingConfig(time_stamp)
    }

    config, err := simplejson.Load(CONFIG_FILE)
    if err != nil {
        return
    }

    id, _ := config.Get("id").String()
    name, _ := config.Get("name").String()
    server, _ := config.Get("server").String()
    key, _ := config.Get("key").String()

    client := http.DefaultClient
    if strings.ToLower(server[:5]) == "https" {
        rootPEM, err := ioutil.ReadFile(PEM_FILE)
        if err != nil {
            fmt.Println("can not load cert.pem:", err)
            os.Exit(1)
        }
        roots := x509.NewCertPool()
        ok := roots.AppendCertsFromPEM(rootPEM)
        if !ok {
            fmt.Println("certificate error")
            os.Exit(1)
        }
        tr := &http.Transport{
            TLSClientConfig:    &tls.Config{RootCAs: roots},
            DisableCompression: true,
        }
        client = &http.Client{Transport: tr}
    }

    stat := GetStat(id, name, time_stamp)
    server = server + "/api/stat"
    key = PassWord(key, stat)

    request, err := http.NewRequest("POST", server, bytes.NewBuffer([]byte(stat)))
    request.Header.Set("X-Client-Key", key)
    request.Header.Set("Content-Type", "application/json")
    request.Header.Set("User-Agent", "Stat Hub API Client/0.1.0 (i@likexian.com)")

    response, err := client.Do(request)
    if err != nil {
        fmt.Println(err)
        os.Exit(1)
    }
    defer response.Body.Close()

    data, _ := ioutil.ReadAll(response.Body)
    text := string(data)
    if text != "" {
        fmt.Println(text)
        os.Exit(1)
    }
}

func SettingConfig(time_stamp int64) {
    host_info, _ := host_stat.GetHostInfo()
    host_name := host_info.HostName

    name := RawInput(fmt.Sprintf("> Please enter the NAME of THIS node [%s]:", host_name), true)
    if name == "" {
        name = host_name
    }

    server := RawInput("> Please enter the URL of SERVER :", false)
    key := RawInput("> Please enter the KEY of SERVER :", false)

    if len(server) <= 7 {
        server = "http://" + server
    }

    if server[:7] != "http://" && server[:8] != "https://" {
        server = "http://" + server
    }

    if server[len(server)-1:] == "/" {
        server = server[:len(server)-1]
    }

    config := Config{}
    config.Server = server
    config.Key = key
    config.Name = name

    random := fmt.Sprintf("%s%s", os.Getpid(), time_stamp)
    config.Id = PassWord(key, random)

    data := simplejson.Json{}
    data.Data = config
    simplejson.Dump(CONFIG_FILE, &data)
}

func GetStat(id string, name string, time_stamp int64) string {
    stat := Stat{}
    stat.Id = id
    stat.TimeStamp = time_stamp

    host_info, _ := host_stat.GetHostInfo()
    stat.OSRelease = host_info.Release + " " + host_info.OSBit
    if name == "" {
        stat.HostName = host_info.HostName
    } else {
        stat.HostName = name
    }

    cpu_info, _ := host_stat.GetCPUInfo()
    stat.CPUName = cpu_info.ModelName
    stat.CPUCore = cpu_info.CoreCount

    cpu_stat, _ := host_stat.GetCPUStat()
    stat.CPURate = Round(100-cpu_stat.IdleRate, 2)

    mem_stat, _ := host_stat.GetMemStat()
    stat.MemRate = mem_stat.MemRate
    stat.SwapRate = mem_stat.SwapRate

    disk_stat, _ := host_stat.GetDiskStat()
    disk_total := uint64(0)
    disk_used := uint64(0)
    for _, v := range disk_stat {
        disk_total += v.Total
        disk_used += v.Used
        if v.UsedRate > 90 {
            stat.DiskWarn += fmt.Sprintf("%s %.2f;", v.Mount, v.UsedRate)
        }
    }
    stat.DiskRate = Round(float64(disk_used)/float64(disk_total), 2)

    io_stat, _ := host_stat.GetIOStat()
    disk_read := uint64(0)
    disk_write := uint64(0)
    for _, v := range io_stat {
        disk_read += v.ReadBytes / 1024
        disk_write += v.WriteBytes / 1024
    }
    stat.DiskRead = disk_read
    stat.DiskWrite = disk_write

    net_stat, _ := host_stat.GetNetStat()
    net_write := uint64(0)
    net_read := uint64(0)
    for _, v := range net_stat {
        if v.Device != "lo" {
            net_write += v.TXBytes / 1024
            net_read += v.RXBytes / 1024
        }
    }
    stat.NetWrite = net_write
    stat.NetRead = net_read

    uptime_stat, _ := host_stat.GetUptimeStat()
    stat.Uptime = uint64(uptime_stat.Uptime)

    load_stat, _ := host_stat.GetLoadStat()
    stat.Load = fmt.Sprintf("%.2f %.2f %.2f", load_stat.LoadNow, load_stat.LoadPre, load_stat.LoadFar)

    json := simplejson.Json{}
    json.Data = stat
    result, _ := simplejson.Dumps(&json)

    return result
}
