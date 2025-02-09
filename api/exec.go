package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/puppetlabs/wash/activity"
	apitypes "github.com/puppetlabs/wash/api/types"
	"github.com/puppetlabs/wash/plugin"
)

// Send serializes an ExecPacket via the provided json encoder.
// Skips if the provided context has been cancelled.
func sendPacket(ctx context.Context, w *json.Encoder, p *apitypes.ExecPacket) {
	select {
	case <-ctx.Done():
		// Don't send anything if the context's finished. Otherwise, the Encode
		// will error w/ a broken pipe.
	default:
		if err := w.Encode(p); err != nil {
			activity.Record(ctx, "Error encoding the packet from %v: %v", p.TypeField, err)
		}
	}
}

// swagger:parameters executeCommand
//nolint:deadcode,unused
type execBody struct {
	// in: body
	Body apitypes.ExecBody
}

// swagger:response
//nolint:deadcode,unused
type execResponse struct {
	// in: body
	Packets []apitypes.ExecPacket
}

// swagger:route POST /fs/exec exec executeCommand
//
// Execute a command on a remote system
//
// Executes a command on the remote system described by the supplied path.
//
//     Consumes:
//     - application/json
//
//     Produces:
//     - application/json
//
//     Schemes: http
//
//     Responses:
//       200: execResponse
//       400: errorResp
//       404: errorResp
//       500: errorResp
var execHandler handler = func(w http.ResponseWriter, r *http.Request) *errorResponse {
	ctx := r.Context()
	entry, path, errResp := getEntryFromRequest(r)
	if errResp != nil {
		return errResp
	}

	if !plugin.ExecAction().IsSupportedOn(entry) {
		return unsupportedActionResponse(path, plugin.ExecAction())
	}

	if r.Body == nil {
		return badActionRequestResponse(path, plugin.ExecAction(), "Please send a JSON request body")
	}

	var body apitypes.ExecBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		return badActionRequestResponse(path, plugin.ExecAction(), err.Error())
	}

	fw, ok := w.(flushableWriter)
	if !ok {
		return unknownErrorResponse(fmt.Errorf("Cannot stream %v, response handler does not support flushing", path))
	}

	activity.Record(ctx, "API: Exec %v %+v", path, body)
	opts := plugin.ExecOptions{}
	if body.Opts.Input != "" {
		opts.Stdin = strings.NewReader(body.Opts.Input)
	}
	cmd, err := plugin.Exec(ctx, entry.(plugin.Execable), body.Cmd, body.Args, opts)
	if err != nil {
		return erroredActionResponse(path, plugin.ExecAction(), err.Error())
	}

	// Ensure every write is a flush, and do an initial flush to send the header.
	w.WriteHeader(http.StatusOK)
	fw.Flush()

	// Stream the command's output
	enc := json.NewEncoder(&streamableResponseWriter{fw})
	for chunk := range cmd.OutputCh() {
		packet := apitypes.ExecPacket{TypeField: chunk.StreamID, Timestamp: chunk.Timestamp}
		if err := chunk.Err; err != nil {
			packet.Err = newStreamingErrorObj(chunk.StreamID, err.Error())
		} else {
			packet.Data = chunk.Data
		}

		sendPacket(ctx, enc, &packet)
	}

	// Now stream its exit code
	packet := apitypes.ExecPacket{TypeField: apitypes.Exitcode, Timestamp: time.Now()}
	exitCode, err := cmd.ExitCode()
	if err != nil {
		packet.Err = newUnknownErrorObj(fmt.Errorf("could not get the exit code: %v", err))
	} else {
		packet.Data = exitCode
	}
	sendPacket(ctx, enc, &packet)

	return nil
}
