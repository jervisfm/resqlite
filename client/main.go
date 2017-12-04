package main

import (
    "bufio"
    "os"
    "os/signal"
    "fmt"
    "bytes"
    "strings"
    "errors"
)



func filter(command string) error {
    var err error

    command = strings.Replace(command, "\n", "", -1)

    // lazy case-insensitive but wtv for now
    upperC := strings.ToUpper(command)

    if strings.Contains(upperC, "RANDOM()") {
        return errors.New("random() or other non-determinstic commands not supported.")
    }

    // TODO(sternhenri): deal with utc modifier
    if (strings.Contains(upperC, "('NOW'") || strings.Contains(upperC, "'NOW')")) {
        return errors.New("now or other non-determinstic commands not supported.")
    }

    if (strings.Contains(upperC, "BEGIN TRANSACTION") ||
        strings.Contains(upperC, "COMMIT TRANSACTION") ||
        strings.Contains(upperC, "END TRANSACTION") ||
        strings.Contains(upperC, "ROLLBACK TRANSACTION")) {
        
        return errors.New("transactions not supported.")
    }

    // doesn't deal with user-defined functions as enabled in
    // SQLite C interface.

    return err
}

func process(command string) string{
    fmt.Print("got %s", command)
    err := filter(command)
    if err != nil {
        return err.Error()
    }

    // TODO(sternhenri): send to master for processing


    return "ok"
}

func repl() {
    //handle signals appropriately; not quite like sqlite3
    c := make(chan os.Signal, 1)
    signal.Notify(c)
    go func() {
        for sig := range c {
            if sig == os.Interrupt {
                os.Exit(0)
            }
        }
    }()

    reader := bufio.NewReader(os.Stdin)
    var buf bytes.Buffer
    exit := false

    for exit != true {
        fmt.Print("resqlite> ")
        text, _ := reader.ReadString('\n')

        if (text == "") {
            // EOF received (^D)
            os.Exit(0);
        }

        if (text == "\n") {
            continue
        }

        for (text == "\n" || text[len(text)-2:] != ";\n") {
            buf.WriteString(text)
            fmt.Print("     ...> ")
            text, _ = reader.ReadString('\n')

            // imitating sqlite3 behavior
            if (text == "") {
                // EOF received (^D)
                exit = true
                break
            }
        }
        buf.WriteString(text)
        output := process(buf.String())
        fmt.Println(output)
        buf.Reset()
    }
}

func main() {
    repl()
}