package relay

// Reconnect interrupts only the current attempt; the channel and outputs stay alive.
func (c *Client) Reconnect() {
	c.connectionMu.Lock()
	defer c.connectionMu.Unlock()
	if c.ctx.Err() != nil {
		return
	}
	c.bumpPending = true
	if c.connectionCancel != nil {
		c.connectionCancel()
	}
}
