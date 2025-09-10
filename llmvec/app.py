import os
from typing import List, Optional, Dict, Any
from fastapi import FastAPI, HTTPException
from pydantic import BaseModel
from qdrant_client import QdrantClient
from qdrant_client.http.models import VectorParams, Distance, PointStruct
from sentence_transformers import SentenceTransformer
from openai import OpenAI
import json
from dotenv import load_dotenv

# Load environment variables from .env file
load_dotenv()

QDRANT_URL = os.getenv("QDRANT_URL", "http://localhost:6333")
COLLECTION_NAME = os.getenv("COLLECTION_NAME", "game_specs")
EMBEDDING_MODEL = os.getenv("EMBEDDING_MODEL", "sentence-transformers/all-MiniLM-L6-v2")
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")

app = FastAPI(title="LLM & Vector Service (Spec Planner)")

# Initialize OpenAI client
if OPENAI_API_KEY:
    openai_client = OpenAI(api_key=OPENAI_API_KEY)
    print(f"OpenAI API key loaded successfully (ends with: ...{OPENAI_API_KEY[-10:]})")
else:
    openai_client = None
    print("Warning: OPENAI_API_KEY not set in environment variables")

model = SentenceTransformer(EMBEDDING_MODEL)
client = QdrantClient(url=QDRANT_URL)

def ensure_collection():
    dim = model.get_sentence_embedding_dimension()
    collections = client.get_collections().collections
    names = {c.name for c in collections}
    if COLLECTION_NAME not in names:
        client.recreate_collection(
            collection_name=COLLECTION_NAME,
            vectors_config=VectorParams(size=dim, distance=Distance.COSINE)
        )

ensure_collection()

class GenSpecReq(BaseModel):
    brief: str
    constraints: Optional[Dict[str, Any]] = None

class GenSpecResp(BaseModel):
    title: str
    spec_markdown: str
    spec_json: Dict[str, Any]

class SearchReq(BaseModel):
    text: str
    top_k: int = 5
    threshold: float = 0.86

class SimilarItem(BaseModel):
    spec_id: str
    title: str
    score: float

class SearchResp(BaseModel):
    similar: List[SimilarItem]

class UpsertReq(BaseModel):
    spec_id: str
    text: str
    payload: Dict[str, Any] = {}

def generate_spec_from_brief(brief: str, constraints: Optional[Dict[str, Any]] = None) -> GenSpecResp:
    if not openai_client:
        raise HTTPException(status_code=500, detail="OpenAI API key not configured")

    try:
        # Construct the prompt for OpenAI
        constraints_text = ""
        if constraints:
            constraints_text = f"\n\nConstraints: {json.dumps(constraints, indent=2)}"

        prompt = f"""Generate a detailed HTML5 web game specification based on the following brief:

Brief: {brief}{constraints_text}

IMPORTANT REQUIREMENTS:
- The game MUST be a WEB-BASED HTML5 game (playable in browsers)
- The game MUST support TWO game modes:
  1. vs AI (single player against computer)
  2. PvP online realtime (multiplayer against other players)
- Design the game mechanics to work seamlessly in both modes
- Include AI behavior specifications for the vs AI mode
- Include networking/synchronization requirements for PvP mode
- Focus on web technologies (HTML5 Canvas, WebGL, WebSockets, etc.)
- BE VERY SPECIFIC about the game concept, mechanics, and gameplay
- Provide detailed descriptions that clearly differentiate this game from others in the same genre

Please respond with a JSON object containing the following structure:
{{
    "title": "Specific Game Title (not just generic names)",
    "description": "Detailed 2-3 sentence description of what makes this game unique and engaging",
    "genre": "specific sub-genre (e.g., 'tower defense', 'match-3 puzzle', 'real-time strategy')",
    "core_concept": "The main game concept and what players do (e.g., 'Players control a spaceship navigating through asteroid fields while collecting power-ups and avoiding enemies')",
    "how_to_play": "Step-by-step explanation of gameplay mechanics, controls, objectives, and win/lose conditions. Be very detailed and specific.",
    "duration_sec": 60,
    "platform": ["web"],
    "controls": ["mouse", "keyboard", "touch"],
    "game_modes": [
        {{
            "mode": "vs_ai",
            "description": "Single player vs computer AI",
            "ai_behavior": "Detailed AI strategy, difficulty progression, and behavioral patterns",
            "gameplay_differences": "How this mode differs from PvP in terms of pacing, difficulty, or mechanics"
        }},
        {{
            "mode": "pvp_online",
            "description": "Real-time multiplayer online",
            "networking": "Specific synchronization requirements and real-time features",
            "competitive_elements": "What makes the PvP experience engaging and competitive"
        }}
    ],
    "detailed_mechanics": [
        {{
            "mechanic_name": "Name of mechanic",
            "description": "Detailed explanation of how this mechanic works",
            "player_interaction": "How players interact with this mechanic"
        }}
    ],
    "objectives": {{
        "primary_goal": "Main objective players must achieve to win",
        "secondary_goals": ["Optional objectives that enhance gameplay"],
        "progression_system": "How players advance or improve during the game"
    }},
    "visual_style": {{
        "art_direction": "Specific visual theme and aesthetic (e.g., 'retro pixel art', 'minimalist geometric', 'cartoon fantasy')",
        "color_palette": "Primary colors and mood",
        "key_visual_elements": ["Important visual components that define the game's look"]
    }},
    "technical_requirements": {{
        "web_technologies": ["HTML5 Canvas", "WebSockets", "JavaScript"],
        "ai_features": "Specific AI implementation details for vs AI mode",
        "networking_features": "Detailed real-time networking requirements for PvP",
        "browser_compatibility": "modern browsers with HTML5 support",
        "performance_considerations": "Key performance requirements and optimizations needed"
    }},
    "game_flow": {{
        "start_sequence": "How the game begins",
        "main_gameplay_loop": "The core repeating cycle of gameplay",
        "end_conditions": "All possible ways the game can end"
    }},
    "unique_features": ["List of distinctive features that set this game apart from similar games"],
    "difficulty_progression": "How the game becomes more challenging over time",
    "assets": [
        {{
            "category": "sprites/sounds/ui",
            "items": ["specific asset names"],
            "description": "What these assets are used for"
        }}
    ]
}}

Ensure the game specification is so detailed that a developer could understand exactly what to build, and the 'how_to_play' section is comprehensive enough for vector similarity search."""

        # Call OpenAI API
        response = openai_client.chat.completions.create(
            model="gpt-3.5-turbo",
            messages=[
                {
                    "role": "system",
                    "content": "You are an expert game designer who creates highly detailed, specific game specifications. Focus on creating unique, well-defined games with clear mechanics and engaging gameplay. Avoid generic descriptions - be specific about what makes each game special and how it plays."
                },
                {
                    "role": "user",
                    "content": prompt
                }
            ],
            max_tokens=3000,
            temperature=0.7
        )

        # Parse the response
        content = response.choices[0].message.content.strip()

        try:
            # Try to parse as JSON
            spec_json = json.loads(content)
        except json.JSONDecodeError:
            # Enhanced fallback with more detailed structure
            spec_json = {
                "title": "Space Debris Collector",
                "description": "A fast-paced arcade game where players pilot a salvage ship through dangerous asteroid fields, collecting valuable space debris while avoiding collisions and enemy drones.",
                "genre": "arcade collection",
                "core_concept": "Navigate a small spacecraft through procedurally generated asteroid fields, collecting glowing debris while managing fuel and avoiding obstacles",
                "how_to_play": "Use arrow keys or WASD to control your spacecraft. Collect glowing debris pieces to score points and refuel. Avoid asteroids and enemy drones that patrol the field. Your fuel depletes over time, so collect fuel canisters to stay alive. The game speeds up as you progress. In vs AI mode, compete against an AI opponent for the highest score. In PvP mode, race against other players in real-time to collect the most debris.",
                "duration_sec": 60,
                "platform": ["web"],
                "controls": ["keyboard", "mouse"],
                "game_modes": [
                    {
                        "mode": "vs_ai",
                        "description": "Single player vs computer AI",
                        "ai_behavior": "AI opponent with adaptive difficulty that learns from player patterns",
                        "gameplay_differences": "AI provides consistent challenge with predictable patterns"
                    },
                    {
                        "mode": "pvp_online",
                        "description": "Real-time multiplayer online",
                        "networking": "WebSocket-based real-time position and score synchronization",
                        "competitive_elements": "Live leaderboard and collision interactions between players"
                    }
                ],
                "detailed_mechanics": [
                    {
                        "mechanic_name": "Debris Collection",
                        "description": "Players collect glowing debris pieces scattered throughout the field",
                        "player_interaction": "Fly over debris to automatically collect it and gain points"
                    }
                ],
                "objectives": {
                    "primary_goal": "Collect the most debris within the time limit",
                    "secondary_goals": ["Avoid all collisions", "Collect fuel efficiently"],
                    "progression_system": "Score multiplier increases with consecutive collections"
                },
                "visual_style": {
                    "art_direction": "retro pixel art space theme",
                    "color_palette": "dark space background with bright neon debris",
                    "key_visual_elements": ["animated spacecraft", "glowing particles", "rotating asteroids"]
                },
                "technical_requirements": {
                    "web_technologies": ["HTML5 Canvas", "WebSockets", "JavaScript"],
                    "ai_features": "Pathfinding AI for computer opponent",
                    "networking_features": "Real-time position synchronization for multiplayer",
                    "browser_compatibility": "modern browsers with HTML5 support",
                    "performance_considerations": "Efficient collision detection and particle systems"
                },
                "game_flow": {
                    "start_sequence": "3-2-1 countdown with spacecraft appearing at spawn point",
                    "main_gameplay_loop": "Navigate, collect, avoid obstacles, manage fuel",
                    "end_conditions": "Time runs out or fuel depleted"
                },
                "unique_features": ["Fuel management system", "Dynamic difficulty scaling", "Particle trail effects"],
                "difficulty_progression": "Asteroid density and enemy drone speed increase over time",
                "assets": [
                    {
                        "category": "sprites",
                        "items": ["spacecraft", "asteroids", "debris", "fuel_canisters"],
                        "description": "Visual game objects and collectibles"
                    }
                ]
            }

        # Generate enhanced markdown with all the detailed information
        title = spec_json.get("title", "Generated Web Game")
        description = spec_json.get("description", "")
        genre = spec_json.get("genre", "arcade")
        core_concept = spec_json.get("core_concept", "")
        how_to_play = spec_json.get("how_to_play", "")
        duration = spec_json.get("duration_sec", 60)
        platform = spec_json.get("platform", ["web"])
        controls = spec_json.get("controls", ["mouse", "keyboard"])
        game_modes = spec_json.get("game_modes", [])
        detailed_mechanics = spec_json.get("detailed_mechanics", [])
        objectives = spec_json.get("objectives", {})
        visual_style = spec_json.get("visual_style", {})
        technical_req = spec_json.get("technical_requirements", {})
        game_flow = spec_json.get("game_flow", {})
        unique_features = spec_json.get("unique_features", [])
        difficulty_progression = spec_json.get("difficulty_progression", "")
        assets = spec_json.get("assets", [])

        spec_markdown = f"""# {title}

## Description
{description}

## Core Concept
{core_concept}

## How to Play
{how_to_play}

## Game Details
- **Genre**: {genre}
- **Platform**: {', '.join(platform)}
- **Duration**: {duration} seconds
- **Controls**: {', '.join(controls)}

## Game Modes
"""

        for mode in game_modes:
            mode_name = mode.get("mode", "unknown")
            description = mode.get("description", "")
            spec_markdown += f"\n### {mode_name.replace('_', ' ').title()}\n{description}\n"

            if "ai_behavior" in mode:
                spec_markdown += f"**AI Behavior**: {mode['ai_behavior']}\n"
            if "networking" in mode:
                spec_markdown += f"**Networking**: {mode['networking']}\n"
            if "gameplay_differences" in mode:
                spec_markdown += f"**Gameplay Differences**: {mode['gameplay_differences']}\n"
            if "competitive_elements" in mode:
                spec_markdown += f"**Competitive Elements**: {mode['competitive_elements']}\n"

        if detailed_mechanics:
            spec_markdown += f"\n## Detailed Mechanics\n"
            for mechanic in detailed_mechanics:
                name = mechanic.get("mechanic_name", "Unknown")
                desc = mechanic.get("description", "")
                interaction = mechanic.get("player_interaction", "")
                spec_markdown += f"\n### {name}\n{desc}\n**Player Interaction**: {interaction}\n"

        if objectives:
            spec_markdown += f"\n## Objectives\n"
            if "primary_goal" in objectives:
                spec_markdown += f"**Primary Goal**: {objectives['primary_goal']}\n"
            if "secondary_goals" in objectives:
                spec_markdown += f"**Secondary Goals**: {', '.join(objectives['secondary_goals'])}\n"
            if "progression_system" in objectives:
                spec_markdown += f"**Progression**: {objectives['progression_system']}\n"

        if visual_style:
            spec_markdown += f"\n## Visual Style\n"
            for key, value in visual_style.items():
                if isinstance(value, list):
                    spec_markdown += f"**{key.replace('_', ' ').title()}**: {', '.join(value)}\n"
                else:
                    spec_markdown += f"**{key.replace('_', ' ').title()}**: {value}\n"

        if game_flow:
            spec_markdown += f"\n## Game Flow\n"
            for key, value in game_flow.items():
                spec_markdown += f"**{key.replace('_', ' ').title()}**: {value}\n"

        if unique_features:
            spec_markdown += f"\n## Unique Features\n"
            for feature in unique_features:
                spec_markdown += f"- {feature}\n"

        if difficulty_progression:
            spec_markdown += f"\n## Difficulty Progression\n{difficulty_progression}\n"

        if technical_req:
            spec_markdown += f"\n## Technical Requirements\n"
            for key, value in technical_req.items():
                if isinstance(value, list):
                    spec_markdown += f"**{key.replace('_', ' ').title()}**: {', '.join(value)}\n"
                else:
                    spec_markdown += f"**{key.replace('_', ' ').title()}**: {value}\n"

        if assets:
            spec_markdown += f"\n## Required Assets\n"
            for asset_group in assets:
                category = asset_group.get("category", "Unknown")
                items = asset_group.get("items", [])
                desc = asset_group.get("description", "")
                spec_markdown += f"\n### {category.title()}\n{desc}\n"
                for item in items:
                    spec_markdown += f"- {item}\n"

        return GenSpecResp(
            title=title,
            spec_markdown=spec_markdown,
            spec_json=spec_json
        )

    except Exception as e:
        print(f"Error generating spec: {str(e)}")
        raise HTTPException(status_code=500, detail=f"Failed to generate spec: {str(e)}")


        # Call OpenAI API using new client syntax
        response = openai_client.chat.completions.create(
            model="gpt-3.5-turbo",
            messages=[
                {
                    "role": "system",
                    "content": "You are a game design expert who creates detailed specifications for HTML5 games with both AI and multiplayer capabilities. Always respond with valid JSON only. Focus on games that work well in both vs AI and PvP online modes."
                },
                {
                    "role": "user",
                    "content": prompt
                }
            ],
            max_tokens=2000,
            temperature=0.7
        )

        # Parse the response using new client syntax
        llm_response = response.choices[0].message.content.strip()

        # Try to extract JSON from the response
        try:
            # Remove any markdown code block formatting if present
            if llm_response.startswith("```json"):
                llm_response = llm_response[7:]
            if llm_response.endswith("```"):
                llm_response = llm_response[:-3]

            spec_json = json.loads(llm_response)
        except json.JSONDecodeError as e:
            print(f"Failed to parse LLM response as JSON: {e}")
            print(f"Raw response: {llm_response}")
            # Fallback to a basic structure with both game modes
            title = (constraints or {}).get("title") or f"Game - {brief[:24]}"
            spec_json = {
                "title": title,
                "genre": (constraints or {}).get("genre", "arcade"),
                "duration_sec": (constraints or {}).get("duration_sec", 60),
                "platform": ["mobile", "desktop"],
                "controls": ["tap", "arrow_keys"],
                "game_modes": [
                    {
                        "mode": "vs_ai",
                        "description": "Single player vs computer AI",
                        "ai_behavior": "Basic AI with adaptive difficulty"
                    },
                    {
                        "mode": "pvp_online",
                        "description": "Real-time multiplayer online",
                        "networking": "WebSocket-based real-time synchronization"
                    }
                ],
                "mechanics": [{"rule": "Generated from brief: " + brief}],
                "win_condition": "Complete the objective",
                "lose_condition": "Fail the objective",
                "assets": {"sprites": "minimal SVG", "audio": "basic sfx"},
                "constraints": {"bundle_kb_max": 600, "fps_min": 50},
                "accessibility": {"high_contrast": True, "pause": True},
                "technical_requirements": {
                    "ai_engine": "Simple state machine for AI behavior",
                    "networking": "WebSocket for real-time communication",
                    "state_sync": "Deterministic game state with conflict resolution"
                }
            }

        # Generate enhanced markdown description
        title = spec_json.get("title", "Untitled Game")
        mechanics_md = "\n".join([f"- {m.get('rule', 'Unknown rule')}" for m in spec_json.get("mechanics", [])])

        # Add game modes section
        game_modes_md = ""
        if "game_modes" in spec_json:
            game_modes_md = "\n## Game Modes\n"
            for mode in spec_json["game_modes"]:
                mode_name = mode.get("mode", "unknown").replace("_", " ").title()
                mode_desc = mode.get("description", "No description")
                game_modes_md += f"### {mode_name}\n- {mode_desc}\n"
                if "ai_behavior" in mode:
                    game_modes_md += f"- AI Behavior: {mode['ai_behavior']}\n"
                if "networking" in mode:
                    game_modes_md += f"- Networking: {mode['networking']}\n"

        spec_md = f"""# {title}

**Brief:** {brief}

**Genre:** {spec_json.get('genre', 'Unknown')}
{game_modes_md}
## Game Mechanics
{mechanics_md}

## Objective
- **Win Condition:** {spec_json.get('win_condition', 'Not specified')}
- **Lose Condition:** {spec_json.get('lose_condition', 'Not specified')}

## Technical Specs
- **Duration:** {spec_json.get('duration_sec', 60)} seconds
- **Platforms:** {', '.join(spec_json.get('platform', []))}
- **Controls:** {', '.join(spec_json.get('controls', []))}

## Assets Required
- **Sprites:** {spec_json.get('assets', {}).get('sprites', 'Not specified')}
- **Audio:** {spec_json.get('assets', {}).get('audio', 'Not specified')}

## Technical Requirements
- **AI Engine:** {spec_json.get('technical_requirements', {}).get('ai_engine', 'Not specified')}
- **Networking:** {spec_json.get('technical_requirements', {}).get('networking', 'Not specified')}
- **State Sync:** {spec_json.get('technical_requirements', {}).get('state_sync', 'Not specified')}
"""

        return GenSpecResp(title=title, spec_markdown=spec_md, spec_json=spec_json)

    except Exception as e:
        print(f"Error calling OpenAI API: {e}")
        # Fallback to placeholder response with both game modes
        title = (constraints or {}).get("title") or f"Game - {brief[:24]}"
        spec_json = {
            "title": title,
            "genre": (constraints or {}).get("genre", "arcade"),
            "duration_sec": (constraints or {}).get("duration_sec", 60),
            "platform": ["mobile", "desktop"],
            "controls": ["tap", "arrow_keys"],
            "game_modes": [
                {
                    "mode": "vs_ai",
                    "description": "Single player vs computer AI",
                    "ai_behavior": "Error generating AI specs - using fallback"
                },
                {
                    "mode": "pvp_online",
                    "description": "Real-time multiplayer online",
                    "networking": "Error generating networking specs - using fallback"
                }
            ],
            "mechanics": [{"rule": "Error generating mechanics - using fallback"}],
            "win_condition": "Complete the objective",
            "lose_condition": "Fail the objective",
            "assets": {"sprites": "minimal SVG", "audio": "basic sfx"},
            "constraints": {"bundle_kb_max": 600, "fps_min": 50},
            "accessibility": {"high_contrast": True, "pause": True},
            "technical_requirements": {
                "ai_engine": "Basic AI implementation",
                "networking": "WebSocket communication",
                "state_sync": "Simple state synchronization"
            }
        }
        spec_md = f"# {title}\n\n**Brief:** {brief}\n\n*Note: Error occurred during LLM generation, using fallback response with dual-mode support.*"
        return GenSpecResp(title=title, spec_markdown=spec_md, spec_json=spec_json)

@app.post("/llm/generate-spec", response_model=GenSpecResp)
def generate_spec(req: GenSpecReq):
    if not req.brief:
        raise HTTPException(status_code=400, detail="brief is required")
    return generate_spec_from_brief(req.brief, req.constraints)

@app.post("/vector/search", response_model=SearchResp)
def search_similar(req: SearchReq):
    ensure_collection()
    emb = model.encode([req.text])[0]
    result = client.search(
        collection_name=COLLECTION_NAME,
        query_vector=emb.tolist(),
        limit=req.top_k,
        with_payload=True,
        score_threshold=req.threshold
    )
    items = []
    for r in result:
        pid = r.id if isinstance(r.id, str) else str(r.id)
        title = r.payload.get("title", "")
        items.append(SimilarItem(spec_id=pid, title=title, score=float(r.score)))
    return SearchResp(similar=items)

@app.post("/vector/upsert")
def upsert_point(req: UpsertReq):
    ensure_collection()
    emb = model.encode([req.text])[0]
    client.upsert(
        collection_name=COLLECTION_NAME,
        points=[PointStruct(id=req.spec_id, vector=emb.tolist(), payload=req.payload)]
    )
    return {"ok": True, "id": req.spec_id}
