import requests
import json
import sys

BASE = 'http://127.0.0.1:19375'

def test_cot():
    print('=== Test 1: Chain of Thought ===')
    r = requests.post(f'{BASE}/api/thinking/cot', json={
        'query': 'If a train travels 120 km in 2 hours, what is its average speed?',
        'user_id': 'test-user',
        'max_steps': 3
    }, timeout=120)
    print(f'Status: {r.status_code}')
    if r.status_code == 200:
        data = r.json()
        steps = data.get('reasoning_steps', [])
        print(f'Reasoning steps: {len(steps)}')
        for step in steps:
            print(f'  Step {step.get("step_number")}: {step.get("step_type")} - {step.get("content", "")[:100]}...')
        print(f'Final answer: {data.get("final_answer", "")[:200]}')
        print(f'Confidence: {data.get("confidence")}')
        return True
    else:
        print(f'Error: {r.text[:300]}')
        return False

def test_multi_step():
    print('\n=== Test 2: Multi-Step Reasoning ===')
    r = requests.post(f'{BASE}/api/thinking/multi-step', json={
        'query': 'Calculate the area of a circle with radius 5',
        'user_id': 'test-user',
        'strategy': 'sequential'
    }, timeout=120)
    print(f'Status: {r.status_code}')
    if r.status_code == 200:
        data = r.json()
        results = data.get('execution_results', [])
        print(f'Execution results: {len(results)}')
        for result in results:
            print(f'  Step {result.get("step_number")}: {result.get("content", "")[:100]}...')
        print(f'Final answer: {data.get("final_answer", "")[:200]}')
        return True
    else:
        print(f'Error: {r.text[:300]}')
        return False

def test_tree_of_thought():
    print('\n=== Test 3: Tree of Thought ===')
    r = requests.post(f'{BASE}/api/thinking/tree-of-thought', json={
        'query': 'What are the main causes of climate change?',
        'user_id': 'test-user',
        'search_strategy': 'best_first',
        'max_depth': 2,
        'branching_factor': 2
    }, timeout=120)
    print(f'Status: {r.status_code}')
    if r.status_code == 200:
        data = r.json()
        nodes = data.get('nodes', [])
        print(f'Nodes explored: {len(nodes)}')
        for node in nodes[:3]:
            print(f'  Node {node.get("id")}: {node.get("content", "")[:80]}...')
        print(f'Best path: {data.get("best_path", [])}')
        return True
    else:
        print(f'Error: {r.text[:300]}')
        return False

def test_thought_crud():
    print('\n=== Test 4: Thought CRUD ===')
    # Create
    r = requests.post(f'{BASE}/api/thinking/thoughts', json={
        'type': 'test',
        'content': 'Test thought content',
        'user_id': 'test-user'
    })
    print(f'Create status: {r.status_code}')
    if r.status_code != 200:
        return False
    thought = r.json()
    thought_id = thought.get('id')
    print(f'Thought ID: {thought_id}')

    # List
    r = requests.get(f'{BASE}/api/thinking/thoughts')
    print(f'List status: {r.status_code}, count: {len(r.json()) if r.status_code == 200 else 0}')

    # Get
    r = requests.get(f'{BASE}/api/thinking/thoughts/{thought_id}')
    print(f'Get status: {r.status_code}')

    # Delete
    r = requests.delete(f'{BASE}/api/thinking/thoughts/{thought_id}')
    print(f'Delete status: {r.status_code}')
    return True

print('=== Thinking Service Test ===')
results = []
results.append(('ChainOfThought', test_cot()))
results.append(('MultiStep', test_multi_step()))
results.append(('TreeOfThought', test_tree_of_thought()))
results.append(('ThoughtCRUD', test_thought_crud()))

print('\n=== Summary ===')
passed = 0
for name, ok in results:
    status = 'PASS' if ok else 'FAIL'
    print(f'  {status}: {name}')
    if ok:
        passed += 1

print(f'\nTotal: {passed}/{len(results)} passed')
