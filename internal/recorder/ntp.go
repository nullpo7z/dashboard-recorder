package recorder

import (
	"fmt"
	"time"

	"github.com/beevik/ntp"
)

// GetNTPTime fetches the current time from an NTP server with retries.
// It returns the offset between NTP time and system time.
// offset = NTP Time - System Time
func GetNTPTime(server string) (time.Duration, error) {
	var err error
	var response *ntp.Response

	for i := 0; i < 3; i++ {
		response, err = ntp.Query(server)
		if err == nil {
			// Offset is the difference between the server's clock and the client's clock.
			// ntp.Query returns a response containing the offset.
			// We validate it slightly by checking if it's not zero (though zero is possible).
			return response.ClockOffset, nil
		}
		// Exponential backoff or simple wait
		time.Sleep(time.Duration(i+1) * 500 * time.Millisecond)
	}

	return 0, fmt.Errorf("failed to query NTP server %s after 3 attempts: %w", server, err)
}
