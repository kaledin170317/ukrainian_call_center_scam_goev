#!/usr/bin/env python3
# generate_test_files.py
import argparse
import os
import random
from datetime import datetime, timedelta, date

TARIFFS_HEADER = "prefix;destination;rate_per_min;connection_fee;timeband;weekday;priority;effective_date;expiry_date"
SUBS_HEADER = "phone_number;client_name"
CDR_DT_FMT = "%Y-%m-%d %H:%M:%S"


def ensure_dir(p: str) -> None:
    os.makedirs(p, exist_ok=True)


def money(x: float) -> str:
    # формат как в примере: 1.80
    return f"{x:.2f}"


def sanitize_account(s: str) -> str:
    # как в примере Office_Billing; для русских имён можно оставить пустым
    t = s.strip().replace(" ", "_")
    return t if all(ord(c) < 128 for c in t) else ""


def gen_tariffs(out_dir: str, eff: date, exp: date) -> str:
    path = os.path.join(out_dir, "tariffs.csv")
    rows = [
        # префикс 7916: высокий приоритет, только будни, только 08:00-20:00
        ("7916", "Москва МТС (мобильный)", money(1.80), money(0.00), "08:00-20:00", "1-5", "100"),
        # общий мобильный 79: всегда, низкий приоритет
        ("79", "Россия (мобильные, общий)", money(2.20), money(0.00), "00:00-00:00", "1-7", "10"),
        # городской Москва 7495
        ("7495", "Москва (городской)", money(0.90), money(0.00), "00:00-00:00", "1-7", "50"),
        # ещё один городской для разнообразия
        ("7499", "Москва (городской, другой)", money(1.05), money(0.00), "00:00-00:00", "1-7", "40"),
        # бесплатный 7800
        ("7800", "800 (бесплатный)", money(0.00), money(0.00), "00:00-00:00", "1-7", "60"),
    ]

    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(TARIFFS_HEADER + "\n")
        for pfx, dst, rpm, conn, tb, wd, pr in rows:
            f.write(f"{pfx};{dst};{rpm};{conn};{tb};{wd};{pr};{eff.isoformat()};{exp.isoformat()}\n")
    return path


def gen_subscribers(out_dir: str, n: int) -> tuple[str, list[tuple[str, str]]]:
    path = os.path.join(out_dir, "subscribers.csv")
    base = 78123260000  # как в примере
    subs: list[tuple[str, str]] = []

    # первый — “Office Billing”, остальные — пользователи
    subs.append((str(base), "Office Billing"))
    for i in range(1, n):
        phone = str(base + i * 37)  # чтобы не просто +1, но стабильно
        name = f"User {i:03d}"
        subs.append((phone, name))

    # добавим одного “русского” для наглядности (имя будет в отчёте)
    if n >= 2:
        subs[1] = (subs[1][0], "Иванов Иван Иванович")

    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(SUBS_HEADER + "\n")
        for phone, name in subs:
            f.write(f"{phone};{name}\n")
    return path, subs


def rand_msisdn(prefix: str, rnd: random.Random, total_len: int = 11) -> str:
    # prefix без '+', итоговое число (например 7916xxxxxxxx)
    rest = total_len - len(prefix)
    return prefix + "".join(str(rnd.randint(0, 9)) for _ in range(rest))


def gen_cdr(out_dir: str, subs: list[tuple[str, str]], lines: int, start_day: date, seed: int) -> str:
    path = os.path.join(out_dir, "cdr.txt")
    rnd = random.Random(seed)

    # базовое время
    base_dt = datetime.combine(start_day, datetime.min.time()) + timedelta(hours=8)

    trunks = ["SIP_Trunk_01", "SIP_Trunk_02", "Local"]
    dispositions = ["answered", "no_answer", "busy", "failed"]

    def pick_sub(i: int) -> tuple[str, str]:
        return subs[i % len(subs)]

    with open(path, "w", encoding="utf-8", newline="\n") as f:
        for i in range(lines):
            caller_phone, caller_name = pick_sub(i)
            account = sanitize_account(caller_name)

            scenario = i % 12

            # время старта: специально делаем кейсы в/вне таймбэнда 7916
            if scenario == 0:
                # внутри 08:00-20:00
                start = base_dt + timedelta(minutes=i)
                called = "+" + rand_msisdn("7916", rnd)
                direction = "outgoing"
                disp = "answered"
            elif scenario == 1:
                # вне таймбэнда (21-23) -> должен уйти на 79
                start = datetime.combine(start_day, datetime.min.time()) + timedelta(hours=22, minutes=i % 60)
                called = "+" + rand_msisdn("7916", rnd)
                direction = "outgoing"
                disp = "answered"
            elif scenario == 2:
                # 7495 городской
                start = base_dt + timedelta(minutes=2 * i)
                called = "+" + rand_msisdn("7495", rnd)
                direction = "outgoing"
                disp = "answered"
            elif scenario == 3:
                # 79 общий мобильный
                start = base_dt + timedelta(minutes=3 * i)
                called = "+" + rand_msisdn("79", rnd)
                direction = "outgoing"
                disp = "answered"
            elif scenario == 4:
                # бесплатный 7800
                start = base_dt + timedelta(minutes=4 * i)
                called = "+" + rand_msisdn("7800", rnd)
                direction = "outgoing"
                disp = "answered"
            else:
                # internal + разные disposition
                start = base_dt + timedelta(minutes=5 * i)
                other_phone, _ = pick_sub(i + 1)
                called = other_phone
                direction = "internal"
                disp = dispositions[rnd.randrange(len(dispositions))]

            # длительность и биллинг
            duration = rnd.randint(10, 260)
            if disp == "answered":
                bill = max(1, duration - rnd.randint(0, 5))
            else:
                bill = 0

            end = start + timedelta(seconds=duration)

            cost_field = "0.00"  # поле в CDR игнорируется у тебя, но оставляем валидным
            call_id = f"call_{i+1:06d}"
            trunk = trunks[rnd.randrange(len(trunks))]

            # Формат: 12+ полей, парсер берёт первые 12
            line = (
                f"{start.strftime(CDR_DT_FMT)}|{end.strftime(CDR_DT_FMT)}|{caller_phone}|{called}|"
                f"{direction}|{disp}|{duration}|{bill}|{cost_field}|{account}|{call_id}|{trunk}"
            )
            f.write(line + "\n")

    return path


def main():
    ap = argparse.ArgumentParser(description="Generate tariffs.csv, subscribers.csv, cdr.txt for billing UI tests.")
    ap.add_argument("--out", default="example", help="Output directory (default: example)")
    ap.add_argument("--seed", type=int, default=42, help="Random seed (default: 42)")
    ap.add_argument("--subscribers", type=int, default=10, help="Number of subscribers (default: 10)")
    ap.add_argument("--cdr-lines", type=int, default=60, help="Number of CDR lines (default: 60)")
    ap.add_argument("--start-date", default="2026-02-03", help="CDR start date YYYY-MM-DD (default: 2026-02-03)")
    args = ap.parse_args()

    ensure_dir(args.out)

    start_day = date.fromisoformat(args.start_date)
    eff = start_day - timedelta(days=30)
    exp = start_day + timedelta(days=365)

    t = gen_tariffs(args.out, eff, exp)
    s, subs = gen_subscribers(args.out, max(1, args.subscribers))
    c = gen_cdr(args.out, subs, max(1, args.cdr_lines), start_day, args.seed)

    print("Generated:")
    print(" -", t)
    print(" -", s)
    print(" -", c)


if __name__ == "__main__":
    main()