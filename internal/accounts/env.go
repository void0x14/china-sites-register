package accounts

import "os"

func lookupEnv(k string) (string, bool) { return os.LookupEnv(k) }
