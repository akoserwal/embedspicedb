package embedspicedb_test

import (
	"fmt"

	"github.com/akoserwal/embedspicedb"
)

func ExampleEmbeddedServer_postgresDatastore() {
	config := embedspicedb.Config{
		DatastoreType: "postgres",
		DatastoreURI:  "postgres://spicedb:secret@localhost:5432/spicedb?sslmode=disable",
	}

	_, err := embedspicedb.New(config)
	fmt.Println(err)
	// Output:
	// failed to create datastore: PostgreSQL datastore is not available in standalone embedspicedb. Use memdb for development, or use embedspicedb within the SpiceDB module context for persistent datastores
}

func ExampleEmbeddedServer_mysqlDatastore() {
	config := embedspicedb.Config{
		DatastoreType: "mysql",
		DatastoreURI:  "spicedb:secret@tcp(localhost:3306)/spicedb?parseTime=true",
	}

	_, err := embedspicedb.New(config)
	fmt.Println(err)
	// Output:
	// failed to create datastore: MySQL datastore is not available in standalone embedspicedb. Use memdb for development, or use embedspicedb within the SpiceDB module context for persistent datastores
}

func ExampleEmbeddedServer_memdbDatastore() {
	config := embedspicedb.DefaultConfig()
	config.DatastoreType = "memdb"

	_, err := embedspicedb.New(config)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Configured with in-memory datastore")
	// Output:
	// Configured with in-memory datastore
}
