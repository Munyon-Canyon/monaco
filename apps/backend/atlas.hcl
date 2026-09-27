env "local" {
  url = getenv("DATABASE_URL")
  dev = "docker://postgres/16/dev"
  migration {
    dir = "file://migrations"
  }
}

env "test" {
  url = getenv("TEST_DATABASE_URL")
  dev = "docker://postgres/16/dev"
  migration {
    dir = "file://migrations"
  }
}
