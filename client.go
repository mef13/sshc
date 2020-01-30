package sshc


import (
	"errors"
	"golang.org/x/crypto/ssh"
	"io"
	"strings"
	"sync"
	"time"
)
var (
	once sync.Once
	ErrIsBusy = errors.New("this host is busy now")
	ErrTimeOut = errors.New("timeout read from ssh")
	ErrChannelClose = errors.New("channel close")
)
type client struct {
	s map[string]*Session
}

type Session struct {
	host string
	w *io.WriteCloser
	r *io.Reader
	s *ssh.Session
	c *ssh.Client
}

type SshOutput struct {
	Number int
	Output string
}

var instance  *client

func get() *client {
	once.Do(func() {
		instance = &client{s:make(map[string]*Session)}
	})
	return instance
}

func Connect(host string, user string, pass string) (*Session, error) {
	c := get()
	_, exist := c.s[host]
	if exist {
		return nil, ErrIsBusy
	}
	sess, err := connect(host, user, pass)
	if err != nil {
		return nil, err
	}
	c.s[host] = sess
	return sess, nil
}

func connect(host string, user string, pass string) (*Session, error)  {
	config := &ssh.ClientConfig{
		User: user,
		Auth: []ssh.AuthMethod{
			ssh.Password(pass),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:time.Second * 15,
	}
	client, err := ssh.Dial("tcp", host, config)
	if err != nil {
		return nil, err
	}
	sess, err := client.NewSession()
	if err != nil {
		return nil, err
	}

	w, err := sess.StdinPipe()
	if err != nil {
		return nil, err
	}
	r, err := sess.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := sess.Shell(); err != nil {
		return nil, err
	}

	return &Session{
		host: host,
		c:client,
		s:sess,
		r: &r,
		w: &w,
	}, nil
}

func (s *Session) Send(msg string) error  {
	wr := *s.w
	_, err := wr.Write([]byte(msg + "\n"))
	if err != nil {
		return err
	}
	return nil
}

func (s *Session) Write(msg string, stopStr ...string) (SshOutput, error) {
	return s.write(msg + "\n", stopStr...)
}

func (s *Session) Writeln(msg string, stopStr ...string) (SshOutput, error) {
	return s.write(msg + "\n\n", stopStr...)
}

func (s *Session) write(msg string, stopStr ...string) (SshOutput, error) {
	wr := *s.w
	_, err := wr.Write([]byte(msg))
	if err != nil {
		return SshOutput{
			Number: -1,
			Output: "",
		}, err
	}
	out := ""
	t := time.NewTimer(20*time.Second)
	defer t.Stop()
	ch := make(chan string)
	go reader(*s.r, ch)
	for {
		select {
		case d := <- ch:
			out += d
			for strNumber, str := range stopStr{
				if strings.Contains(out, str) {
					return SshOutput{
						Number: strNumber,
						Output: out,
					}, nil
				}
			}
			go reader(*s.r, ch)
		case <- t.C:
			return SshOutput{
				Number: -1,
				Output: out,
			}, ErrTimeOut
		}
	}
}

func reader(r io.Reader, out chan<- string)  {
	var buf = make([]byte, 64 * 1024)
	n, err := r.Read(buf)
	if err != nil {
		return
	}
	out <- string(buf[:n])
}


func (s *Session) Wait() error {
	return s.s.Wait()
}

func (s *Session) Close()  {
	s.c.Close()
	s.s.Close()
	c := get()
	delete(c.s, s.host)
}

