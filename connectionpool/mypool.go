package connectionpool

import (
	"errors"
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	// ErrClosed is error which pool has been closed but still been used
	ErrClosed = errors.New("pool has been closed")
	// ErrNil is error which pool is nil but has been used
	ErrNil = errors.New("pool is nil")
	// Get connection error
	ErrGetConn = errors.New("get connection error")
)

type MyConn struct {
	Conn          net.Conn
	LastVisitTime int64
	Invalid       bool // if a connection is broken, this flag is true
}

// PoolConfig used for config the connection pool
type PoolConfig struct {
	// Init capacity of the connection pool
	InitCap int
	// Maxcap is the max connection number of the pool
	MaxCap int
	// method can get a connection
	Factory func() (*MyConn, error)
	// WaitTimeout is the timeout for waiting to get a connection
	WaitTimeout time.Duration
	// IdleTimeout is the timeout for a connection to be alive
	IdleTimeout time.Duration
}

func InitPoolConfig(initCap int, maxCap int, factory Factory, waitTimeout time.Duration, idleTimeout time.Duration) (*PoolConfig, error) {
	config := PoolConfig{
		InitCap:     initCap,
		MaxCap:      maxCap,
		Factory:     factory,
		WaitTimeout: waitTimeout,
		IdleTimeout: idleTimeout,
	}
	return &config, nil
}

// MyPool store connections and pool info
type MyPool struct {
	// use Channel to contain connections
	conns       chan *MyConn
	factory     Factory
	mutex       sync.RWMutex
	poolConfig  *PoolConfig
	idleConns   int
	activityNum int //已经创建并且活着并且在池子里的连接数
}

// Factory generate a new connection
type Factory func() (*MyConn, error)

// NewGPool create a connection pool
// the initial connection number is poolConfig.InitCap
func NewMyPool(poolConfig *PoolConfig) (*MyPool, error) {
	// test initCap and maxCap
	if poolConfig.InitCap < 0 || poolConfig.MaxCap < 0 || poolConfig.InitCap > poolConfig.MaxCap {
		return nil, errors.New("invalid capacity setting")
	}
	myPool := &MyPool{
		factory:    poolConfig.Factory,
		poolConfig: poolConfig,
		idleConns:  poolConfig.InitCap, // 初始化的时候，idle connections等于初始化的connections数
		conns:      make(chan *MyConn, poolConfig.MaxCap),
	}

	// create initial connection, if wrong just close it
	for i := 0; i < poolConfig.InitCap; i++ {
		myConn, err := poolConfig.Factory()
		if err != nil {
			myPool.Close()
			//myPool.addRemainingSpace()
			return nil, fmt.Errorf("factory is not able to fill the pool: %s", err)
		}
		// put the connection to the channel
		myPool.conns <- myConn
	}
	myPool.activityNum = poolConfig.InitCap
	return myPool, nil
}

// getConnsAndFactory get conn channel and factory by once
func (myPool *MyPool) getConnsAndFactory() (chan *MyConn, Factory) {
	myPool.mutex.RLock()
	conns := myPool.conns
	factory := myPool.factory
	myPool.mutex.RUnlock()
	return conns, factory
}

// Return return the connection back to the pool. If the pool is full or closed,
// conn is simply closed. A nil conn will be rejected.
func (myPool *MyPool) Return(myConn *MyConn) error {
	//log.Println("here------------0")
	if myConn == nil {
		//log.Println("here------------4")
		return errors.New("connection is nil. rejecting")
	}

	if myConn.Invalid {
		myPool.mutex.Lock()
		myPool.activityNum--
		myPool.mutex.Unlock()
		return errors.New("connection invalid, no need to return to pool. rejecting")
	}

	if myPool.conns == nil {
		// pool is closed, close passed connection
		//log.Println("here------------1")
		return myConn.Conn.Close()
	}

	// close idle timeout connections, when the conn is timeout, we can use it, but after using, we close it.
	idleTime := time.Now().Unix() - myConn.LastVisitTime
	milliseconds := myPool.poolConfig.IdleTimeout.Milliseconds()
	if idleTime > milliseconds {
		myPool.mutex.Lock()
		myPool.activityNum--
		myPool.mutex.Unlock()
		return myConn.Conn.Close()
	}

	// put the resource back into the pool. If the pool is full, this will
	// block and the default case will be executed.
	select {
	case myPool.conns <- myConn:
		myPool.mutex.Lock()
		myPool.idleConns++
		myPool.mutex.Unlock()
		// if we succeed in using a connection and it is not timeout, we refresh the last visit time
		myConn.LastVisitTime = time.Now().Unix()
		//log.Println("here------------2,idle: ", myPool.idleConns)
		return nil
	default:
		// pool is full, close passed connection
		//log.Println("here------------3,idle: ", myPool.idleConns)
		return myConn.Conn.Close()
	}

}

// Get implement Pool get interface
// if don't have any connection available, it will try to new one
func (myPool *MyPool) Get() (*MyConn, error) {
	conns, factory := myPool.getConnsAndFactory()
	if conns == nil {
		return nil, ErrNil
	}
	select {
	case conn := <-conns:
		if conn == nil {
			return nil, ErrClosed
		}
		myPool.mutex.Lock()
		myPool.idleConns--
		myPool.mutex.Unlock()
		return conn, nil
	default:
		if myPool.activityNum >= myPool.poolConfig.MaxCap {
			// connections run out, get until time out
			select {
			case conn := <-conns:
				if conn == nil {
					return nil, ErrClosed
				}
				myPool.mutex.Lock()
				myPool.idleConns--
				myPool.mutex.Unlock()
				return conn, nil
			case <-time.After(myPool.poolConfig.WaitTimeout):
				return nil, errors.New("more than max capacity and get connection timeout")
			}
		}
		myConn, err := factory()
		if err != nil {
			return nil, err
		}
		myPool.mutex.Lock()
		defer myPool.mutex.Unlock()
		myPool.activityNum++
		return myConn, nil
	}
}

// Close implement Pool close interface
// it will close all the connection in the pool
func (myPool *MyPool) Close() {
	myPool.mutex.Lock()
	conns := myPool.conns
	myPool.conns = nil
	myPool.factory = nil
	myPool.mutex.Unlock()

	if conns == nil {
		return
	}

	close(conns)
	for conn := range conns {
		conn.Conn.Close()
	}
}

// Len implement Pool Len interface
// it will return current length of the pool
func (myPool *MyPool) Len() int {
	conns, _ := myPool.getConnsAndFactory()
	return len(conns)
}

// Idle implement Pool Idle interface
// it will return current idle length of the pool
func (myPool *MyPool) Idle() int {
	myPool.mutex.Lock()
	defer myPool.mutex.Unlock()
	idle := myPool.idleConns
	return idle
}
