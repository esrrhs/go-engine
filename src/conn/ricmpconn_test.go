package conn

import (
	"fmt"
	"github.com/oliver256/go-engine/src/loggo"
	"strconv"
	"testing"
	"time"
)

func Test000RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen(":58083")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		cc.Accept()
		fmt.Println("accept done")
	}()

	time.Sleep(time.Second)

	cc.Close()

	time.Sleep(time.Second)
}

func Test0002RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		conn, err := c.Dial("9.9.9.9")
		fmt.Println("Dial return")
		if err != nil {
			fmt.Println(err)
		} else {
			fmt.Println(conn.Info())
		}

	}()

	time.Sleep(time.Second)

	c.Close()
	fmt.Println("closed")

	time.Sleep(time.Second)
}

func Test0003RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		cc.Accept()
		fmt.Println("accept done")
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		buf := make([]byte, 100)
		_, err := ccc.Read(buf)
		if err != nil {
			fmt.Println(err)
			return
		}
	}()

	time.Sleep(time.Second * 5)
	fmt.Println("start close listener")
	cc.Close()
	fmt.Println("close listener ok")
	fmt.Println("start close client")
	ccc.Close()
	fmt.Println("close client ok")

	time.Sleep(time.Second)
}

func Test0004RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		cc.Accept()
		fmt.Println("accept done")
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		buf := make([]byte, 1000)
		for i := 0; i < 10000; i++ {
			_, err := ccc.Write(buf)
			if err != nil {
				fmt.Println(err)
				return
			}
		}
		fmt.Println("write done")
	}()

	time.Sleep(time.Second)

	cc.Close()
	ccc.Close()

	time.Sleep(time.Second)
}

func Test0005RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	exit := false

	go func() {
		cc, err := cc.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		defer cc.Close()
		fmt.Println("accept done")
		buf := make([]byte, 10)
		for !exit {
			n, err := cc.Read(buf)
			if err != nil {
				fmt.Println(err)
				fmt.Println("Read done")
				return
			}
			fmt.Println(string(buf[0:n]))
			time.Sleep(time.Millisecond * 100)
		}
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		for i := 0; i < 10000 && !exit; i++ {
			_, err := ccc.Write([]byte("hahaha" + strconv.Itoa(i)))
			if err != nil {
				fmt.Println(err)
				return
			}
		}
		fmt.Println("write done")
	}()

	time.Sleep(time.Second * 5)

	cc.Close()
	ccc.Close()

	exit = true

	time.Sleep(time.Second)
}

func Test0005RICMP1(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	exit := false

	go func() {
		cc, err := cc.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("accept done")
		for i := 0; i < 10000 && !exit; i++ {
			_, err := cc.Write([]byte("hahaha" + strconv.Itoa(i)))
			if err != nil {
				fmt.Println(err)
				return
			}
		}
		fmt.Println("write done")
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		buf := make([]byte, 10)
		for !exit {
			n, err := ccc.Read(buf)
			if err != nil {
				fmt.Println(err)
				fmt.Println("Read done")
				return
			}
			fmt.Println(string(buf[0:n]))
			time.Sleep(time.Millisecond * 100)
		}
		fmt.Println("write done")
	}()

	time.Sleep(time.Second)

	cc.Close()
	ccc.Close()

	exit = true

	time.Sleep(time.Second)
}

func Test0006RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		cc, err := cc.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		defer cc.Close()
		fmt.Println("accept done")
		buf := make([]byte, 10)
		_, err = cc.Read(buf)
		if err != nil {
			fmt.Println("Read " + err.Error())
			return
		}
		fmt.Println("Read done")

	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		time.Sleep(time.Second)
		ccc.Close()
		fmt.Println("client close")
	}()

	time.Sleep(time.Second * 20)

	fmt.Println("start close")
	cc.Close()
	ccc.Close()

	time.Sleep(time.Second)
}

func Test0007RICMP(t *testing.T) {

	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		cc, err := cc.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		defer cc.Close()
		fmt.Println("accept done")
		buf := make([]byte, 10)
		_, err = cc.Read(buf)
		if err != nil {
			fmt.Println("Read " + err.Error())
			return
		}
		fmt.Println("Read done")
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	time.Sleep(time.Second * 5)

	fmt.Println("start close")
	cc.Close()
	ccc.Close()

	time.Sleep(time.Second)
}

func Test0008RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("0.0.0.0")
	if err != nil {
		fmt.Println(err)
		return
	}

	exit := false

	go func() {
		cc, err := cc.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("accept done")
		data := make([]byte, 1024*1024)
		start := time.Now()
		speed := 0
		for !exit {
			//fmt.Println("start Write")
			_, err := cc.Write(data)
			if err != nil {
				fmt.Println(err)
				return
			}
			//fmt.Println("end Write")
			speed += len(data)
			if time.Now().Sub(start) > time.Second {
				speed = speed / 1024 / 1024
				loggo.Info("write speed %v MB per second", float64(speed)/float64(time.Now().Sub(start)/time.Second))
				speed = 0
				start = time.Now()
			}
		}
		fmt.Println("write done")
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		fmt.Println("start client")
		buf := make([]byte, 1024*1024)
		start := time.Now()
		speed := 0
		for !exit {
			//fmt.Println("start Read")
			n, err := ccc.Read(buf)
			if err != nil {
				fmt.Println(err)
				fmt.Println("Read done")
				return
			}
			//fmt.Println("end Read")
			speed += n
			if time.Now().Sub(start) > time.Second {
				speed = speed / 1024 / 1024
				loggo.Info("read speed %v MB per second", float64(speed)/float64(time.Now().Sub(start)/time.Second))
				speed = 0
				start = time.Now()
			}
		}
		fmt.Println("write done")
	}()

	time.Sleep(time.Second * 10)

	cc.Close()
	ccc.Close()

	exit = true

	time.Sleep(time.Second)
}

func Test0009RICMP(t *testing.T) {
	c, err := NewConn("ricmp")
	if err != nil {
		fmt.Println(err)
		return
	}

	cc, err := c.Listen("0.0.0.0")
	if err != nil {
		fmt.Println(err)
		return
	}

	exit := false

	go func() {
		cc, err := cc.Accept()
		if err != nil {
			fmt.Println(err)
			return
		}
		fmt.Println("accept done")
		buf := make([]byte, 1024*1024)
		start := time.Now()
		speed := 0
		for !exit {
			//fmt.Println("start Read")
			n, err := cc.Read(buf)
			if err != nil {
				fmt.Println(err)
				fmt.Println("Read done")
				return
			}
			//fmt.Println("end Read")
			speed += n
			if time.Now().Sub(start) > time.Second {
				speed = speed / 1024 / 1024
				loggo.Info("read speed %v MB per second", float64(speed)/float64(time.Now().Sub(start)/time.Second))
				speed = 0
				start = time.Now()
			}
		}
		fmt.Println("write done")
	}()

	ccc, err := c.Dial("127.0.0.1")
	if err != nil {
		fmt.Println(err)
		return
	}

	go func() {
		fmt.Println("start client")
		data := make([]byte, 1024*1024)
		start := time.Now()
		speed := 0
		for !exit {
			//fmt.Println("start Write")
			_, err := ccc.Write(data)
			if err != nil {
				fmt.Println(err)
				return
			}
			//fmt.Println("end Write")
			speed += len(data)
			if time.Now().Sub(start) > time.Second {
				speed = speed / 1024 / 1024
				loggo.Info("write speed %v MB per second", float64(speed)/float64(time.Now().Sub(start)/time.Second))
				speed = 0
				start = time.Now()
			}
		}
		fmt.Println("write done")
	}()

	time.Sleep(time.Second * 10)

	cc.Close()
	ccc.Close()

	exit = true

	time.Sleep(time.Second)
}
