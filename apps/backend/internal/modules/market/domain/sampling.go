package domain

import "time"

const SampleBucket = 2 * time.Minute

func Bucket(at time.Time) time.Time { return at.Truncate(SampleBucket) }

type Source string

const SourceJupiter Source = "jupiter"
