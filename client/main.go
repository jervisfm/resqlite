package main

import (
    "bufio"
    "os"
    "os/signal"
    "fmt"
    "bytes"
    "strings"
    "errors"
    // "flag"
    // "regexp"

    rsql "github.com/jervisfm/resqlite/server"
)

func Filter(command string) error {
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

func Process(command string) string{
    fmt.Print("got %s", command)
    err := Filter(command)
    if err != nil {
        return err.Error()
    }

    // TODO: (sternhenri) will need to use regexp.Split in order to not split strings containing ;
    var buf bytes.Buffer
    comms := strings.Split(command, ";")
    for _, com := range comms {
        if (len(com) > 1 && text[:1] == ".") {
            return "sqlite3 .* syntax not supported."
        }

        output, err := rsql.ExecCommand(com)
        if err != nil {
            return err.Error()
        }
        buf.WriteString(output)
    }

    return buf.String()
}

func Repl() {
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
        buf.WriteString(strings.TrimSpace(text))
        output := Process(buf.String())
        fmt.Println(output)
        buf.Reset()
    }
}

// Unfortunately, the Sqlite3 cli forces us to take in the db name at launch,
// otherwise tracking state would be quite a mess.
func ParseArgs() string {
    // dbName = flag.String("database", "", "The database file on which the commands will be run.")
    // dbLoc = flag.String("location", "../data/", "The relative path to the database file.")

    //  if (dbLoc && dbLoc[len(dbLoc)-1:] != "/") {
    //     dbLoc += "/"
    // }
    
    // flag.Parse()

    args := os.Args
    if len(args) < 0 {
        panic("Please provide a path to your db file.")
    }

    return args[1]
}

func main() {
    db := ParseArgs()
    rsql.CacheDb(db)
    Repl()
}