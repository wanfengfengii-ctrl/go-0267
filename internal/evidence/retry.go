package evidence

// RetryBase is the fixed logical-time spacing between consecutive attempts.
// Because logical time advances deterministically with task mutations, the
// next attempt instant is always computable from the current one.
const RetryBase int64 = 1

// NextRetry computes the next attempt number and its logical "next at" instant
// for a device retry. The attempt counter strictly increases and the next
// instant equals the base logical time plus the attempt index, yielding a
// stable, monotonic schedule (domain rule 6).
//
// The retry key itself is the task, device, evidence kind, measured object and
// generation tuple, which the persistence layer encodes as the device_attempt
// primary key so retries stay deterministic across restarts.
func NextRetry(logicalTime int64, attempt int) (nextAttempt int, nextAt int64) {
	nextAttempt = attempt + 1
	nextAt = logicalTime + RetryBase*int64(nextAttempt)
	return nextAttempt, nextAt
}
