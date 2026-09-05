import random
from datetime import datetime, timedelta

START_TIME = datetime(2026, 1, 1, 10, 0, 0)
OPERATORS = {
    "MTS": ["913", "983"],
    "Megafon": ["923", "929"],
    "Beeline": ["903", "905", "906"],
    "Tele2": ["952", "953"],
    "Yota": ["999"]
}
CALL_TYPES = ["voice_out", "voice_in", "sms_in", "sms_out", "data_lte", "data_3g", "voice_fail"]
CELL_IDS = ["A100", "A105", "B200", "C300", "D400", "E500", "F600"]
BASE_LAT = 55.0188
BASE_LON = 82.9339

def generate_phone(operator):
    prefix = random.choice(OPERATORS[operator])
    suffix = "".join([str(random.randint(0, 9)) for _ in range(7)])
    return f"7{prefix}{suffix}"

def generate_imsi():
    return f"250{random.randint(10, 99)}00000{random.randint(10000, 99999)}"

POPULATION_SIZE = 1000
population = []
for _ in range(POPULATION_SIZE):
    op = random.choice(list(OPERATORS.keys()))
    population.append({
        "imsi": generate_imsi(),
        "msisdn": generate_phone(op)
    })

print("timestamp,imsi,msisdn,call_type,duration_sec,cell_id,tower_lat,tower_lon")
for i in range(100000):
    timestamp = (START_TIME + timedelta(seconds=i/10 + random.randint(0, 60))).strftime("%Y-%m-%dT%H:%M:%SZ")
    subscriber = random.choice(population)
    call_type = random.choice(CALL_TYPES)
    duration = random.randint(0, 3600) if "voice" in call_type or "data" in call_type else 0
    if call_type == "voice_fail":
        duration = 0
    
    cell_id = random.choice(CELL_IDS)
    lat = round(BASE_LAT + random.uniform(-0.01, 0.01), 4)
    lon = round(BASE_LON + random.uniform(-0.01, 0.01), 4)
    
    print(f"{timestamp},{subscriber['imsi']},{subscriber['msisdn']},{call_type},{duration},{cell_id},{lat},{lon}")
