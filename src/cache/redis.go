package cache

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// RedisCache implements Cache interface using Redis
type RedisCache struct {
	config     *Config
	conn       net.Conn
	mu         sync.Mutex
	reader     *bufio.Reader
	connected  bool
	stats      Stats
}

// NewRedisCache creates a new Redis cache
func NewRedisCache(cfg *Config) (*RedisCache, error) {
	c := &RedisCache{
		config: cfg,
		stats:  Stats{Backend: "redis"},
	}

	if err := c.connect(); err != nil {
		return nil, err
	}

	return c, nil
}

// connect establishes Redis connection
func (c *RedisCache) connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var err error
	c.conn, err = net.DialTimeout("tcp", c.config.Address, 5*time.Second)
	if err != nil {
		return fmt.Errorf("failed to connect to Redis: %w", err)
	}

	c.reader = bufio.NewReader(c.conn)

	// Authenticate if password is set
	if c.config.Password != "" {
		if err := c.sendCommand("AUTH", c.config.Password); err != nil {
			c.conn.Close()
			return fmt.Errorf("failed to authenticate: %w", err)
		}
		if _, err := c.readResponse(); err != nil {
			c.conn.Close()
			return fmt.Errorf("authentication failed: %w", err)
		}
	}

	// Select database
	if c.config.DB > 0 {
		if err := c.sendCommand("SELECT", strconv.Itoa(c.config.DB)); err != nil {
			c.conn.Close()
			return fmt.Errorf("failed to select database: %w", err)
		}
		if _, err := c.readResponse(); err != nil {
			c.conn.Close()
			return fmt.Errorf("database selection failed: %w", err)
		}
	}

	c.connected = true
	c.stats.Connected = true
	return nil
}

// sendCommand sends a Redis command
func (c *RedisCache) sendCommand(args ...string) error {
	cmd := fmt.Sprintf("*%d\r\n", len(args))
	for _, arg := range args {
		cmd += fmt.Sprintf("$%d\r\n%s\r\n", len(arg), arg)
	}
	_, err := c.conn.Write([]byte(cmd))
	return err
}

// readResponse reads a Redis response
func (c *RedisCache) readResponse() (interface{}, error) {
	line, err := c.reader.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(line, "\r\n")

	if len(line) == 0 {
		return nil, fmt.Errorf("empty response")
	}

	switch line[0] {
	case '+': // Simple string
		return line[1:], nil
	case '-': // Error
		return nil, fmt.Errorf("redis error: %s", line[1:])
	case ':': // Integer
		return strconv.ParseInt(line[1:], 10, 64)
	case '$': // Bulk string
		length, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if length == -1 {
			return nil, nil // Nil value
		}
		data := make([]byte, length+2)
		_, err = c.reader.Read(data)
		if err != nil {
			return nil, err
		}
		return data[:length], nil
	case '*': // Array
		count, err := strconv.Atoi(line[1:])
		if err != nil {
			return nil, err
		}
		if count == -1 {
			return nil, nil
		}
		result := make([]interface{}, count)
		for i := 0; i < count; i++ {
			result[i], err = c.readResponse()
			if err != nil {
				return nil, err
			}
		}
		return result, nil
	default:
		return nil, fmt.Errorf("unknown response type: %c", line[0])
	}
}

// Get retrieves a value from Redis
func (c *RedisCache) Get(ctx context.Context, key string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		if err := c.connect(); err != nil {
			c.stats.Misses++
			return nil, err
		}
	}

	if err := c.sendCommand("GET", key); err != nil {
		c.stats.Misses++
		return nil, err
	}

	resp, err := c.readResponse()
	if err != nil {
		c.stats.Misses++
		return nil, err
	}

	if resp == nil {
		c.stats.Misses++
		return nil, fmt.Errorf("key not found: %s", key)
	}

	c.stats.Hits++
	return resp.([]byte), nil
}

// Set stores a value in Redis with TTL
func (c *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		if err := c.connect(); err != nil {
			return err
		}
	}

	if ttl > 0 {
		if err := c.sendCommand("SETEX", key, strconv.Itoa(int(ttl.Seconds())), string(value)); err != nil {
			return err
		}
	} else {
		if err := c.sendCommand("SET", key, string(value)); err != nil {
			return err
		}
	}

	_, err := c.readResponse()
	return err
}

// Delete removes a value from Redis
func (c *RedisCache) Delete(ctx context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		if err := c.connect(); err != nil {
			return err
		}
	}

	if err := c.sendCommand("DEL", key); err != nil {
		return err
	}

	_, err := c.readResponse()
	return err
}

// Exists checks if a key exists in Redis
func (c *RedisCache) Exists(ctx context.Context, key string) (bool, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		if err := c.connect(); err != nil {
			return false, err
		}
	}

	if err := c.sendCommand("EXISTS", key); err != nil {
		return false, err
	}

	resp, err := c.readResponse()
	if err != nil {
		return false, err
	}

	count, ok := resp.(int64)
	if !ok {
		return false, fmt.Errorf("unexpected response type")
	}
	return count > 0, nil
}

// Clear removes all keys matching a pattern
func (c *RedisCache) Clear(ctx context.Context, pattern string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		if err := c.connect(); err != nil {
			return err
		}
	}

	// Use KEYS to find matching keys (for small datasets)
	// For production with large datasets, consider SCAN
	if err := c.sendCommand("KEYS", pattern); err != nil {
		return err
	}

	resp, err := c.readResponse()
	if err != nil {
		return err
	}

	keys, ok := resp.([]interface{})
	if !ok || len(keys) == 0 {
		return nil
	}

	// Delete all matching keys
	args := make([]string, len(keys)+1)
	args[0] = "DEL"
	for i, k := range keys {
		args[i+1] = string(k.([]byte))
	}

	if err := c.sendCommand(args...); err != nil {
		return err
	}

	_, err = c.readResponse()
	return err
}

// Close closes the Redis connection
func (c *RedisCache) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.connected = false
	c.stats.Connected = false
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// Ping checks Redis connectivity
func (c *RedisCache) Ping(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected {
		if err := c.connect(); err != nil {
			return err
		}
	}

	if err := c.sendCommand("PING"); err != nil {
		return err
	}

	resp, err := c.readResponse()
	if err != nil {
		return err
	}

	if resp != "PONG" {
		return fmt.Errorf("unexpected ping response: %v", resp)
	}
	return nil
}

// Stats returns cache statistics
func (c *RedisCache) Stats(ctx context.Context) (*Stats, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	stats := c.stats // Copy base stats

	if !c.connected {
		return &stats, nil
	}

	// Get DBSIZE for key count
	if err := c.sendCommand("DBSIZE"); err == nil {
		if resp, err := c.readResponse(); err == nil {
			if count, ok := resp.(int64); ok {
				stats.Keys = count
			}
		}
	}

	// Get INFO memory for memory usage
	if err := c.sendCommand("INFO", "memory"); err == nil {
		if resp, err := c.readResponse(); err == nil {
			if info, ok := resp.([]byte); ok {
				lines := strings.Split(string(info), "\r\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "used_memory:") {
						mem, _ := strconv.ParseInt(strings.TrimPrefix(line, "used_memory:"), 10, 64)
						stats.MemoryUsed = mem
						break
					}
				}
			}
		}
	}

	return &stats, nil
}
