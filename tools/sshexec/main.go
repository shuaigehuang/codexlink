package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

func main() {
	host := flag.String("host", "", "SSH host and port")
	user := flag.String("user", "ubuntu", "SSH user")
	keyPath := flag.String("key", "", "private key path")
	passwordEnv := flag.String("password-env", "SSH_PASSWORD", "environment variable containing the SSH password")
	knownHostsPath := flag.String("known-hosts", "", "known_hosts path")
	insecureHostKey := flag.Bool("insecure-host-key", false, "skip host key verification")
	flag.Parse()

	if *host == "" || flag.NArg() == 0 {
		fmt.Fprintln(os.Stderr, "usage: sshexec -host HOST [-key KEY | -password-env ENV] [-known-hosts FILE | -insecure-host-key] run|upload ...")
		os.Exit(2)
	}

	var auth []ssh.AuthMethod
	if *keyPath != "" {
		key, err := os.ReadFile(*keyPath)
		must(err)
		signer, err := ssh.ParsePrivateKey(key)
		if _, ok := err.(*ssh.PassphraseMissingError); ok {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(key, []byte(os.Getenv("SSH_KEY_PASSPHRASE")))
		}
		must(err)
		auth = append(auth, ssh.PublicKeys(signer))
	}
	if password := os.Getenv(*passwordEnv); password != "" {
		auth = append(auth, ssh.Password(password))
	}
	if len(auth) == 0 {
		fmt.Fprintln(os.Stderr, "no SSH authentication method configured")
		os.Exit(2)
	}

	var hostKeyCallback ssh.HostKeyCallback
	if *insecureHostKey {
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	} else {
		if *knownHostsPath == "" {
			fmt.Fprintln(os.Stderr, "-known-hosts is required unless -insecure-host-key is set")
			os.Exit(2)
		}
		var err error
		hostKeyCallback, err = knownhosts.New(*knownHostsPath)
		must(err)
	}

	client, err := ssh.Dial("tcp", *host, &ssh.ClientConfig{
		User:            *user,
		Auth:            auth,
		HostKeyCallback: hostKeyCallback,
		Timeout:         15 * time.Second,
	})
	must(err)
	defer client.Close()

	switch flag.Arg(0) {
	case "run":
		if flag.NArg() != 2 {
			fmt.Fprintln(os.Stderr, "usage: sshexec ... run COMMAND")
			os.Exit(2)
		}
		must(run(client, flag.Arg(1)))
	case "upload":
		if flag.NArg() != 3 {
			fmt.Fprintln(os.Stderr, "usage: sshexec ... upload LOCAL REMOTE")
			os.Exit(2)
		}
		must(upload(client, flag.Arg(1), flag.Arg(2)))
	default:
		fmt.Fprintf(os.Stderr, "unknown action %q\n", flag.Arg(0))
		os.Exit(2)
	}
}

func run(client *ssh.Client, command string) error {
	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	return session.Run(command)
}

func upload(client *ssh.Client, localPath, remotePath string) error {
	file, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}

	session, err := client.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}
	session.Stdout = os.Stdout
	session.Stderr = os.Stderr
	if err := session.Start("cat > " + shellQuote(remotePath)); err != nil {
		return err
	}
	written, err := io.Copy(stdin, file)
	if err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}
	if err := session.Wait(); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "uploaded %d of %d bytes to %s\n", written, info.Size(), remotePath)
	return nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func must(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
