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
