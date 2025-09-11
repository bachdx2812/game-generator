import os
from pathlib import Path
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
EMBEDDING_MODEL = os.getenv(
    "EMBEDDING_MODEL", "sentence-transformers/all-MiniLM-L6-v2")
OPENAI_API_KEY = os.getenv("OPENAI_API_KEY")

app = FastAPI(title="LLM & Vector Service (Spec Planner)")

# Initialize OpenAI client
if OPENAI_API_KEY:
    openai_client = OpenAI(api_key=OPENAI_API_KEY)
    print(
        f"OpenAI API key loaded successfully (ends with: ...{OPENAI_API_KEY[-10:]})")
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


def load_spec_prompt_template() -> str:
    """Load the detailed spec prompt template from llm_backend directory"""
    current_dir = Path(__file__).parent
    prompt_path = current_dir / "spec_prompt.txt"

    try:
        if prompt_path.exists():
            return prompt_path.read_text(encoding='utf-8')
        else:
            print(f"Warning: spec_prompt.txt not found at {prompt_path}")
            # Fallback to a basic prompt if file not found
            return """Generate a detailed game specification for {GAME_NAME}.

Brief: {BRIEF}

Provide a comprehensive specification including:
            - Overview and core mechanics
            - Platform requirements (mobile-first)
            - Game modes (vs AI and PvP online)
            - Controls and UI requirements
            - Win/lose conditions
            - Technical implementation notes

Respond with a detailed markdown specification."""
    except Exception as e:
        print(f"Error loading spec_prompt.txt: {e}")
        return "Generate a detailed game specification based on the brief: {BRIEF}"


def generate_spec_from_brief(brief: str, constraints: Optional[Dict[str, Any]] = None) -> GenSpecResp:
    if not openai_client:
        raise HTTPException(
            status_code=500, detail="OpenAI API key not configured")

    try:
        # Load the detailed prompt template
        prompt_template = load_spec_prompt_template()

        # Extract game name from brief or use a default
        game_name = brief.split('.')[0].strip() if '.' in brief else brief[:50]

        # Replace placeholders in the template
        prompt = prompt_template.replace("{GAME_NAME}", game_name)
        prompt = prompt.replace("{BRIEF}", brief)

        # Add blockchain chain placeholder (you can customize this)
        prompt = prompt.replace("{BLOCKCHAIN_CHAIN}", "Ethereum")

        # Add constraints if provided
        if constraints:
            constraints_text = f"\n\nAdditional Constraints: {json.dumps(constraints, indent=2)}"
            prompt += constraints_text

        response = openai_client.chat.completions.create(
            model="gpt-4",
            messages=[
                {
                    "role": "system",
                    "content": "You are a Requirements Author specializing in game specifications. You MUST respond with valid JSON only, following the exact format specified in the prompt."
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
        llm_response = response.choices[0].message.content.strip()

        # Clean up potential markdown formatting
        if llm_response.startswith("```json"):
            llm_response = llm_response[7:]
        if llm_response.endswith("```"):
            llm_response = llm_response[:-3]
        llm_response = llm_response.strip()

        try:
            # Parse the structured JSON response
            parsed_response = json.loads(llm_response)

            if "markdown" in parsed_response and "json" in parsed_response:
                spec_markdown = parsed_response["markdown"]
                spec_json = parsed_response["json"]
                title = spec_json.get("title", game_name)
            else:
                raise ValueError(
                    "Response missing required 'markdown' or 'json' fields")

        except (json.JSONDecodeError, ValueError) as e:
            print(f"Failed to parse structured JSON response: {e}")
            # Fallback to original parsing logic
            json_start = llm_response.rfind('{')
            json_end = llm_response.rfind('}') + 1

            if json_start >= 0 and json_end > json_start:
                json_text = llm_response[json_start:json_end]
                spec_json = json.loads(json_text)
                spec_markdown = llm_response[:json_start].strip()
            else:
                # Final fallback JSON structure
                title = game_name
                spec_json = {}
                spec_markdown = llm_response

        return GenSpecResp(
            title=spec_json.get("title", game_name),
            spec_markdown=spec_markdown,
            spec_json=spec_json
        )

    except Exception as e:
        print(f"Error in generate_spec_from_brief: {e}")
        # Return fallback response
        title = brief[:50] if brief else "Generated Game"
        fallback_json = {}

        spec_md = f"# {title}\n\n**Brief:** {brief}\n\n*Note: Error occurred during generation, using fallback response.*"
        return GenSpecResp(title=title, spec_markdown=spec_md, spec_json=fallback_json)


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
        items.append(SimilarItem(
            spec_id=pid, title=title, score=float(r.score)))
    return SearchResp(similar=items)


@app.post("/vector/upsert")
def upsert_point(req: UpsertReq):
    ensure_collection()
    emb = model.encode([req.text])[0]
    client.upsert(
        collection_name=COLLECTION_NAME,
        points=[PointStruct(
            id=req.spec_id, vector=emb.tolist(), payload=req.payload)]
    )
    return {"ok": True, "id": req.spec_id}


# Add these new models after the existing ones
class GenCodeReq(BaseModel):
    game_spec: Dict[str, Any]
    output_format: str = "files"  # "files" or "zip"


class GeneratedFile(BaseModel):
    path: str
    content: str
    file_type: str


class GenCodeResp(BaseModel):
    success: bool
    files: List[GeneratedFile]
    project_structure: Dict[str, Any]
    build_instructions: str
    error: Optional[str] = None


def generate_code_from_spec(game_spec: Dict[str, Any]) -> GenCodeResp:
    if not openai_client:
        return GenCodeResp(
            success=False,
            files=[],
            project_structure={},
            build_instructions="",
            error="OpenAI API key not configured"
        )

    try:
        # Extract key information from GameSpec
        genre = game_spec.get("genre", "arcade")
        mechanics = game_spec.get("mechanics", [])
        controls = game_spec.get("controls", ["arrow_keys"])
        title = game_spec.get("title", "Game")

        # Extract essential game rules from spec_markdown
        spec_markdown = game_spec.get('spec_markdown', '')

        # Parse key sections from the markdown
        essential_info = {
            'overview': '',
            'rules': '',
            'win_conditions': '',
            'controls': ''
        }

        if spec_markdown:
            lines = spec_markdown.split('\n')
            current_section = None

            for line in lines:
                line_lower = line.lower().strip()

                # Identify key sections
                if 'overview' in line_lower and line.startswith('#'):
                    current_section = 'overview'
                elif any(keyword in line_lower for keyword in ['rules', 'legal moves', 'gameplay']) and line.startswith('#'):
                    current_section = 'rules'
                elif any(keyword in line_lower for keyword in ['win', 'lose', 'draw', 'victory']) and line.startswith('#'):
                    current_section = 'win_conditions'
                elif 'control' in line_lower and line.startswith('#'):
                    current_section = 'controls'
                elif line.startswith('#') and current_section:
                    current_section = None  # New section, stop collecting

                # Collect content for current section (limit to prevent token overflow)
                if current_section and not line.startswith('#') and line.strip():
                    if len(essential_info[current_section]) < 300:  # Limit each section
                        essential_info[current_section] += line + '\n'

        # Create optimized prompt focusing on implementation
        prompt = f"""Generate a complete HTML5 game: {title}

Game Title: {title}
Game Type: {genre}
Core Mechanics: {', '.join(mechanics[:3]) if mechanics else 'standard game mechanics'}
Controls: {', '.join(controls)}

Game Overview:
{essential_info['overview'][:200]}

Key Rules:
{essential_info['rules'][:300]}

Win/Lose Conditions:
{essential_info['win_conditions'][:200]}

CRITICAL: You MUST respond with ONLY a valid JSON object. Do NOT include any explanatory text, markdown formatting, or code blocks. Start your response directly with {{ and end with }}.

Required JSON format:
{{
  "success": true,
  "files": [
    {{"path": "index.html", "content": "[COMPLETE HTML CODE]", "file_type": "html"}},
    {{"path": "style.css", "content": "[COMPLETE CSS CODE]", "file_type": "css"}},
    {{"path": "game.js", "content": "[COMPLETE JAVASCRIPT CODE]", "file_type": "javascript"}}
  ],
  "project_structure": {{}},
  "build_instructions": "Open index.html in browser"
}}

Requirements:
- Vanilla HTML5/CSS3/JavaScript only
- Canvas-based with requestAnimationFrame game loop
- Mobile-responsive with touch controls
- Complete game state management
- Working collision detection and physics
- Sound effects and visual feedback
- Score system and game over/restart
- Implement ALL specified game mechanics
- Clean, readable code with proper error handling

IMPORTANT:
1. Generate complete working code, not placeholders!
2. Your response must be valid JSON that can be parsed directly
3. Do NOT wrap your response in markdown code blocks
4. Do NOT include any text before or after the JSON object"""

        print(f"[DEBUG] Optimized prompt length: {len(prompt)} characters")

        try:
            response = openai_client.chat.completions.create(
                model="gpt-4",
                messages=[
                    {"role": "system", "content": "You are a professional game developer. You MUST respond with valid JSON only. Do not include any explanatory text, markdown formatting, or code blocks. Your response must start with { and end with }."},
                    {"role": "user", "content": prompt}
                ],
                max_tokens=4000,
                temperature=0.3
            )
        except Exception as api_error:
            print(f"[ERROR] OpenAI API call failed: {api_error}")
            return GenCodeResp(
                success=False,
                files=[],
                project_structure={},
                build_instructions="",
                error=f"OpenAI API call failed: {str(api_error)}"
            )

        print(f"[DEBUG] OpenAI API response: {response}")

        if not response.choices or not response.choices[0].message.content:
            print("[ERROR] Empty response from OpenAI")
            return GenCodeResp(
                success=False,
                files=[],
                project_structure={},
                build_instructions="",
                error="Empty response from OpenAI API - check API key and credits"
            )

        content = response.choices[0].message.content.strip()
        print(f"[DEBUG] OpenAI response length: {len(content)} characters")
        print(f"[DEBUG] OpenAI response preview: {content[:200]}...")

        # Enhanced JSON extraction with multiple fallback strategies
        def extract_json_from_response(text):
            """Extract JSON from LLM response with multiple strategies"""
            text = text.strip()

            # Strategy 1: Direct JSON parsing (ideal case)
            if text.startswith('{') and text.endswith('}'):
                try:
                    return json.loads(text)
                except json.JSONDecodeError:
                    pass

            # Strategy 2: Remove markdown code blocks
            if '```json' in text:
                start = text.find('```json') + 7
                end = text.find('```', start)
                if end > start:
                    json_text = text[start:end].strip()
                    try:
                        return json.loads(json_text)
                    except json.JSONDecodeError:
                        pass

            # Strategy 3: Find JSON object boundaries
            json_start = text.find('{')
            json_end = text.rfind('}') + 1
            if json_start >= 0 and json_end > json_start:
                json_text = text[json_start:json_end]
                try:
                    return json.loads(json_text)
                except json.JSONDecodeError:
                    pass

            # Strategy 4: Remove common prefixes/suffixes
            prefixes_to_remove = [
                "Here is", "Here's", "```json", "```", "The JSON", "JSON:"
            ]
            suffixes_to_remove = [
                "```", "Let me know", "Hope this helps"
            ]

            cleaned_text = text
            for prefix in prefixes_to_remove:
                if cleaned_text.lower().startswith(prefix.lower()):
                    cleaned_text = cleaned_text[len(prefix):].strip()

            for suffix in suffixes_to_remove:
                if cleaned_text.lower().endswith(suffix.lower()):
                    cleaned_text = cleaned_text[:-len(suffix)].strip()

            # Try parsing cleaned text
            json_start = cleaned_text.find('{')
            json_end = cleaned_text.rfind('}') + 1
            if json_start >= 0 and json_end > json_start:
                json_text = cleaned_text[json_start:json_end]
                try:
                    return json.loads(json_text)
                except json.JSONDecodeError:
                    pass

            return None

        # Parse the JSON response with enhanced extraction
        try:
            parsed_response = extract_json_from_response(content)

            if parsed_response is None:
                raise json.JSONDecodeError(
                    "Could not extract valid JSON", content, 0)

            print(
                f"[DEBUG] Successfully parsed JSON response with {len(parsed_response.get('files', []))} files")

            # Validate the response structure
            if not isinstance(parsed_response, dict) or not parsed_response.get('success'):
                raise ValueError("Invalid response structure")

            files = parsed_response.get('files', [])
            if not files:
                raise ValueError("No files generated")

            # Convert to our response format
            generated_files = []
            for file_data in files:
                generated_files.append(GeneratedFile(
                    path=file_data['path'],
                    content=file_data['content'],
                    file_type=file_data['file_type']
                ))

            return GenCodeResp(
                success=True,
                files=generated_files,
                project_structure=parsed_response.get('project_structure', {}),
                build_instructions=parsed_response.get(
                    'build_instructions', 'Open index.html in any modern web browser to play'),
                error=None
            )

        except json.JSONDecodeError as e:
            print(f"[ERROR] Failed to parse LLM response as JSON: {e}")
            print(f"[ERROR] Raw content: {content[:500]}...")

            # Fallback: try to extract JSON from text
            json_start = content.find('{')
            json_end = content.rfind('}') + 1
            if json_start >= 0 and json_end > json_start:
                try:
                    extracted_json = content[json_start:json_end]
                    parsed_response = json.loads(extracted_json)
                    print("[DEBUG] Successfully extracted JSON from text response")

                    files = parsed_response.get('files', [])
                    generated_files = []
                    for file_data in files:
                        generated_files.append(GeneratedFile(
                            path=file_data['path'],
                            content=file_data['content'],
                            file_type=file_data['file_type']
                        ))

                    return GenCodeResp(
                        success=True,
                        files=generated_files,
                        project_structure=parsed_response.get(
                            'project_structure', {}),
                        build_instructions=parsed_response.get(
                            'build_instructions', 'Open index.html in any modern web browser to play'),
                        error=None
                    )
                except json.JSONDecodeError:
                    pass

            # Final fallback with minimal working files
            return GenCodeResp(
                success=False,
                files=[],
                project_structure={},
                build_instructions="",
                error=f"Failed to parse LLM response: {str(e)}"
            )

    except Exception as e:
        print(f"[ERROR] Exception in generate_code_from_spec: {e}")
        return GenCodeResp(
            success=False,
            files=[],
            project_structure={},
            build_instructions="",
            error=f"Code generation failed: {str(e)}"
        )

# Add this endpoint after the existing ones


@app.post("/llm/generate-code", response_model=GenCodeResp)
def generate_code(req: GenCodeReq):
    try:
        if not req.game_spec:
            return GenCodeResp(
                success=False,
                files=[],
                project_structure={},
                build_instructions="",
                error="game_spec is required"
            )

        result = generate_code_from_spec(req.game_spec)

        # Ensure we always return a valid response
        if not result:
            return GenCodeResp(
                success=False,
                files=[],
                project_structure={},
                build_instructions="",
                error="Failed to generate code - no result returned"
            )

        return result

    except Exception as e:
        print(f"[ERROR] Exception in generate_code endpoint: {e}")
        return GenCodeResp(
            success=False,
            files=[],
            project_structure={},
            build_instructions="",
            error=f"Code generation failed: {str(e)}"
        )


@app.delete("/vector/clear")
def clear_vector_db():
    """Clear all vectors from the collection"""
    try:
        # Delete the collection entirely
        client.delete_collection(collection_name=COLLECTION_NAME)

        # Recreate the empty collection
        ensure_collection()

        return {"ok": True, "message": f"Collection '{COLLECTION_NAME}' cleared successfully"}
    except Exception as e:
        raise HTTPException(
            status_code=500, detail=f"Failed to clear collection: {str(e)}")


@app.delete("/vector/collection")
def recreate_collection():
    """Recreate the entire collection (alternative method)"""
    try:
        dim = model.get_sentence_embedding_dimension()
        client.recreate_collection(
            collection_name=COLLECTION_NAME,
            vectors_config=VectorParams(size=dim, distance=Distance.COSINE)
        )
        return {"ok": True, "message": f"Collection '{COLLECTION_NAME}' recreated successfully"}
    except Exception as e:
        raise HTTPException(
            status_code=500, detail=f"Failed to recreate collection: {str(e)}")


@app.delete("/vector/spec/{spec_id}")
def delete_spec_from_vector(spec_id: str):
    """Delete a specific spec from the vector database"""
    try:
        # Delete the point with the given spec_id
        client.delete(
            collection_name=COLLECTION_NAME,
            points_selector=[spec_id]
        )
        return {"ok": True, "message": f"Spec '{spec_id}' deleted from vector database successfully"}
    except Exception as e:
        raise HTTPException(
            status_code=500, detail=f"Failed to delete spec from vector database: {str(e)}")
