package main

import (
	"github.com/gorilla/mux"
	"net"
	"net/http"
	"ray.li/entrytaskclient/conf"
	"ray.li/entrytaskclient/connectionpool"
	"ray.li/entrytaskclient/controller"
	"ray.li/entrytaskserver/server"
	"time"
)

var factory = func() (*connectionpool.MyConn, error) {
	config := conf.GetGlobalConfig().TCPConfig
	conn, err := net.Dial("tcp4", config.Host+config.Port)
	if err != nil {
		return nil, connectionpool.ErrGetConn
	}
	myconn := &connectionpool.MyConn{
		Conn:          conn,
		LastVisitTime: time.Now().Unix(),
		Invalid:       false,
	}
	return myconn, nil
}

func main() {
	// init global config variable
	if err := conf.InitializeConfig(); err != nil {
		server.ExitOnError(err)
	}

	config := conf.GetGlobalConfig()

	// init pool config
	poolConfig := config.ConnPoolConfig
	if poolCfg, err := connectionpool.InitPoolConfig(poolConfig.InitCap, poolConfig.MaxCap, factory, time.Duration(poolConfig.WaitTimeout), time.Duration(poolConfig.IdleTimeout)); err == nil {
		connectionpool.PoolCfg = poolCfg
	}
	// init connection pool
	if pool, err := connectionpool.NewMyPool(connectionpool.PoolCfg); err == nil {
		connectionpool.Pool = pool
	}

	r := mux.NewRouter()
	r.HandleFunc("/index", controller.Index).Methods("GET")
	// add methods control to prevent CSRF post atteck
	r.HandleFunc("/login", controller.LoginWithoutRpc).Methods("POST")
	r.HandleFunc("/updateNickNameResult", controller.UpdateNickName).Methods("POST")
	// static file server
	r.PathPrefix("/static").Handler(http.StripPrefix("/static", http.FileServer(http.Dir("static/"))))

	http.ListenAndServe(":80", r)
}
