/*
 * Copyright 1999-2020 Alibaba Group Holding Ltd.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package nacos

const (
	StateRunning = iota
	StateShutdown
)

type Instance struct {
	InstanceId                string            `json:"instanceId"`
	Ip                        string            `json:"ip"`
	Port                      uint64            `json:"port"`
	Weight                    float64           `json:"weight"`
	Healthy                   bool              `json:"healthy"`
	Enable                    bool              `json:"enabled"`
	Ephemeral                 bool              `json:"ephemeral"`
	ClusterName               string            `json:"clusterName"`
	ServiceName               string            `json:"serviceName"`
	Metadata                  map[string]string `json:"metadata"`
	InstanceHeartBeatInterval int               `json:"instanceHeartBeatInterval"`
	IpDeleteTimeout           int               `json:"ipDeleteTimeout"`
	InstanceHeartBeatTimeOut  int               `json:"instanceHeartBeatTimeOut"`
}

type Service struct {
	CacheMillis              uint64     `json:"cacheMillis"`
	Hosts                    []Instance `json:"hosts"`
	Checksum                 string     `json:"checksum"`
	LastRefTime              uint64     `json:"lastRefTime"`
	Clusters                 string     `json:"clusters"`
	Name                     string     `json:"name"`
	GroupName                string     `json:"groupName"`
	Valid                    bool       `json:"valid"`
	AllIPs                   bool       `json:"allIPs"`
	ReachProtectionThreshold bool       `json:"reachProtectionThreshold"`
}
