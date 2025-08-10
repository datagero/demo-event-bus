# Engine Alternatives

This directory contains alternative implementations of the message processing engine for educational purposes and future reference.

## Python Implementation (`python/`)

The original Python-based worker implementation using threading and the `pika` library.

**Files:**
- `bus.py` - Python worker threads with RabbitMQ consumer logic
- `event_bus.py` - RabbitMQ client abstraction for Python

**Characteristics:**
- **Concurrency**: Python threading with GIL limitations
- **Performance**: ~100-500 messages/sec per worker (depends on message complexity)
- **Memory**: Higher memory usage due to Python overhead
- **Ecosystem**: Rich Python ecosystem, easy debugging
- **Educational Value**: Simpler to understand for Python developers

## Migration Notes

The system was migrated to Go-only workers for:
- **Performance**: 5-10x throughput improvement
- **Concurrency**: True parallelism with goroutines
- **Memory Efficiency**: Lower memory footprint
- **Maintenance**: Single language for workers reduces complexity

## Future Considerations

The Python implementation could be restored for:
- **Polyglot Learning**: Demonstrating multiple language approaches
- **Performance Comparison**: Side-by-side benchmarking
- **Specific Use Cases**: When Python ecosystem benefits outweigh performance costs

To restore Python workers, move files back to `app/` and update `web_server.py` to include the hybrid system integration.