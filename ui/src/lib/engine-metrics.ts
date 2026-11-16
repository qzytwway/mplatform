// The names each Engine reports a metric under, joined for __name__=~ so one
// query covers vLLM and SGLang services alike.
export const ENGINE_METRICS = {
  promptTokens: 'vllm:prompt_tokens_total|sglang:prompt_tokens_total',
  generationTokens:
    'vllm:generation_tokens_total|sglang:generation_tokens_total',
  firstTokenBuckets:
    'vllm:time_to_first_token_seconds_bucket|sglang:time_to_first_token_seconds_bucket',
  runningRequests: 'vllm:num_requests_running|sglang:num_running_reqs',
  queuedRequests: 'vllm:num_requests_waiting|sglang:num_queue_reqs',
  kvCacheUsage: 'vllm:kv_cache_usage_perc|sglang:token_usage',
}
