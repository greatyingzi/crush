package ace

// reflectionPromptTemplate is ported from
// agentic_context_engineering/src/prompts/reflection.txt.
const reflectionPromptTemplate = `# Task
Extract key points from reasoning trajectories, classify them with concise tags, and evaluate existing playbook key points.

# Reasoning Trajectories
{trajectories}

# Current Playbook (existing, above divider)
{existing_playbook}

# Pending / new key points (below divider)
{pending_playbook}

# Existing Tags in Playbook
{existing_tags_context}

# Instructions
1. Key points should have reference value for future, such as:
   - Failed approaches to avoid
   - Successful solutions and patterns
   - User preferences and habits
   - Solutions with measurable impact (performance, reliability, security)
   - Patterns that reduce development time or complexity
   - Critical edge cases or pitfalls to avoid
   - User-specific preferences that affect solution selection

2. From Reasoning Trajectories, derive NEW candidate points that are NOT already covered.

3. **FOR NEW KEY POINTS - Calculate Initial Score from Multi-Dimensional Assessment:**
   - Base score = (effect_rating * 2) + (innovation_level * 1) - (risk_level * 1)
   - Round to nearest integer, range: -2 to +3
   - Examples:
     * effect:0.8, risk:-0.9, innovation:0.2 → score = 1.6 + 0.2 + 0.9 = 2.7 ≈ 3
     * effect:0.4, risk:0.0, innovation:0.9 → score = 0.8 + 0.9 + 0.0 = 1.7 ≈ 2
     * effect:0.2, risk:-0.3, innovation:0.5 → score = 0.4 + 0.5 + 0.3 = 1.2 ≈ 1

4. **FOR EACH NEW KEY POINT - Extract Multi-Dimensional Assessment:**
   - effect_rating (0-1): How effective is this solution? (0.0=ineffective, 0.5=moderate, 1.0=highly_effective)
   - risk_level (-1-0): How risky is this approach? (-1.0=very_safe, -0.5=moderately_safe, 0.0=neutral_risk)
   - innovation_level (0-1): How innovative is this? (0.0=standard_practice, 0.5=clever_approach, 1.0=breakthrough_innovation)
   - Examples:
     * "Use try-catch for error handling" → effect:0.8, risk:-0.9, innovation:0.2
     * "Implement quantum cryptography for authentication" → effect:0.4, risk:0.0, innovation:0.9

4. **TAG RESPONSIBILITY - Your Core Task for Tags:**
   - You are responsible for ALL tag decisions - no automatic merging will happen later
   - Analyze ALL existing tags from the "Existing Tags in Playbook" section above
   - When creating new key points, first check if any existing tags (80%+ semantic similarity) are suitable
   - If existing tags are suitable, REUSE them instead of creating new ones
   - If multiple similar tags exist, choose the MOST FREQUENTLY USED one (listed first)
   - Only create new tags when absolutely necessary for concepts not covered by existing tags
   - Examples: If you see "python" and "python3" in existing tags, prefer "python"; if "api" exists, use it instead of "apis"

4. Normalize → cluster → merge across BOTH sets (existing + pending + new candidates):
   - For any potential cluster, estimate semantic similarity; merge only when similarity is ≥ 0.8 (80%).
   - When merging tags, consolidate to most frequently used existing tag
   - Cluster items that share the same actor/subject + goal/action + outcome/constraint
   - For each admitted cluster, merge all unique details into ONE canonical, action-oriented sentence (≤180 characters)
   - Examples:
     - Input cluster: "Use retries for flaky API calls" + "For unstable endpoints, add retry with backoff" → Output: "Use retry with backoff for flaky/unstable API endpoints to reduce transient failures."
     - Input cluster: "Cache expensive queries" + "Memoize repeated DB reads" → Output: "Cache/memoize expensive, repeated DB queries to cut latency and load."

5. Cross-check with Current Playbook:
   - If a candidate's meaning is already present, skip it unless you add genuinely new nuance.
   - When nuance exists, merge it into a single line rather than adding a duplicate entry.

6. Keep only compact, self-contained sentences; avoid fluff, hedging, or multi-sentence items. All key point text and tags must be concise English.

7. **FINAL TAG ASSIGNMENT (Your Responsibility):**
   - Assign 2-5 concise, lowercase tags that describe topic, technology, or intent
   - **PRIORITY:** Always reuse existing tags when semantically similar (≥80%)
   - **CONSOLIDATION:** When similar tags exist, use most frequently used one
   - **DUPLICATE AVOIDANCE:** Never create "apis" if "api" exists; never create "python3" if "python" exists
   - sources: list of original key point names that were merged (may include existing and pending names). Keep this list concise.

8. Evaluate EACH existing playbook key point based on reasoning trajectories:
   - "highly_effective": key point was extremely useful, providing significant value (score +3)
   - "moderately_useful": key point was helpful and worked as expected (score +2)
   - "slightly_useful": key point had minor positive impact (score +1)
   - "neutral": key point was not relevant or had no impact (score +0)
   - "slightly_harmful": key point caused minor issues or confusion (score -1)
   - "moderately_harmful": key point provided misleading guidance or caused problems (score -2)
   - "highly_dangerous": key point was seriously harmful, causing major issues (score -4)

9. **OUTPUT OPTIMIZATION - Only report changes:**
   - For evaluations, ONLY output key points that need score changes (any non-neutral rating)
   - DO NOT output "neutral" evaluations - they don't affect scores
   - This reduces token usage significantly

   Use the expanded 7-level rating system for more granular feedback:
   - Positive: "highly_effective", "moderately_useful", "slightly_useful"
   - Negative: "slightly_harmful", "moderately_harmful", "highly_dangerous"

# Output Format
{
  "merged_key_points": [
    {
      "text": "Canonical merged key point",
      "tags": ["python", "api"],
      "sources": ["kpt_001", "kpt_042"],
      "effect_rating": 0.8,
      "risk_level": -0.5,
      "innovation_level": 0.3
    },
    {
      "text": "Another canonical merged key point",
      "tags": ["testing"],
      "sources": ["pending_kpt_003"],
      "effect_rating": 0.9,
      "risk_level": -0.8,
      "innovation_level": 0.1
    }
  ],
  "evaluations": [
    {"name": "kpt_001", "rating": "highly_effective"},
    {"name": "kpt_002", "rating": "moderately_harmful"},
    {"name": "kpt_003", "rating": "slightly_useful"}
  ]
}
`
