#!/usr/bin/env python3
"""Small, quota-aware acceptance check. Python 3 standard library only."""
import argparse
import datetime
import getpass
import json
import math
import statistics
import sys
import time
import urllib.error
import urllib.request
import urllib.parse
import uuid

PLAN = {"goal":"endurance", "workout_type":"cardio", "duration_minutes":30,
        "experience_level":"beginner", "environment":"outdoor", "space":"unrestricted",
        "available_equipment":["bodyweight"], "preferred_activity_ids":["act_walk_outdoor"],
        "seed":42, "save":False}

def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--base', default='https://nizdri.stei.fun')
    p.add_argument('--partner', action='store_true', help='Prompt for a full-scope test partner key')
    p.add_argument('--demo-email', help='Synthetic test account email')
    p.add_argument('--register-demo', action='store_true', help='Create the synthetic account once')
    p.add_argument('--write-demo-workout', action='store_true', help='Create synthetic saved workout and activity log')
    p.add_argument('--benchmark', action='store_true', help='Measure 12 sequential requests per route, local STB only')
    args = p.parse_args()
    if (args.register_demo or args.write_demo_workout) and not args.demo_email:
        p.error('demo flags require --demo-email')
    if args.benchmark and urllib.parse.urlparse(args.base).hostname not in ('127.0.0.1', 'localhost', '::1'):
        p.error('--benchmark must target the local STB gateway')
    base=args.base.rstrip('/')
    fail=[]
    def call(label, path, method='GET', payload=None, expected=200, headers=None, verify=None, quiet=False, raw=False):
        h={'Accept':'application/json'}
        if headers:h.update(headers)
        data=None
        if payload is not None:
            h['Content-Type']='application/json';data=json.dumps(payload).encode('utf-8')
        start=time.monotonic()
        try:
            req=urllib.request.Request(base+path, data=data, headers=h, method=method)
            try:
                resp=urllib.request.urlopen(req, timeout=7)
            except urllib.error.HTTPError as e:
                resp=e
            with resp:
                status=resp.code; buf=resp.read(); reqid=resp.headers.get('X-Request-Id','-')
            ms=(time.monotonic()-start)*1000
            if status!=expected:
                raise ValueError('HTTP {} expected {}; request_id={}'.format(status,expected,reqid))
            obj=buf if raw else (json.loads(buf.decode('utf-8')) if buf else None)
            if verify is not None and not verify(obj):
                raise ValueError('response assertion failed; request_id='+reqid)
            if not quiet: print('PASS {:36} HTTP {} {:7.1f} ms'.format(label,status,ms),flush=True)
            return obj,ms
        except Exception as e:
            fail.append(label)
            # Never echo body, request data, or headers (may contain credentials).
            print('FAIL {}: {}'.format(label, str(e) if isinstance(e,ValueError) else type(e).__name__),flush=True)
            return None,None
    def has_data(o):return isinstance(o,dict) and 'data' in o
    def items(o):return [i for section in o['data']['sections'] for i in section['items']]
    def valid_plan(o):
        if not has_data(o):return False
        d=o['data']; ids=[i['item_id'] for i in items(o)]
        return bool(ids) and len(ids)==len(set(ids)) and d['estimated_duration_seconds']<=1800 and d['saved'] is False
    print('Target: '+base,flush=True)
    call('health','/health',verify=lambda o:o['status']=='ok')
    call('readiness','/ready',verify=lambda o:o['status']=='ready')
    call('OpenAPI','/openapi.json',verify=lambda o:o['openapi'].startswith('3.1') and '/calories/recommendations' in o['paths'])
    call('Swagger HTML','/docs',raw=True,verify=lambda o:b'SwaggerUIBundle' in o)
    call('exercise pagination','/api/v1/exercises?limit=2',verify=lambda o:len(o['data'])==2 and o['meta']['pagination']['total']>=53)
    call('exercise details','/api/v1/exercises/ex_bodyweight_squat',verify=has_data)
    call('equipment','/api/v1/equipment',verify=has_data)
    call('muscles','/api/v1/muscle-groups',verify=has_data)
    call('gym list','/api/v1/gyms?limit=20',verify=lambda o:o['meta']['pagination']['total']>=9 and all('demo' not in g['id'] for g in o['data']))
    call('gym details','/api/v1/gyms/gym_bandung_saraga_itb',verify=has_data)
    call('gym equipment','/api/v1/gyms/gym_bandung_saraga_itb/equipment',verify=has_data)
    call('gym facilities','/api/v1/gyms/gym_bandung_saraga_itb/facilities',verify=has_data)
    est='/api/v1/calories/estimate?activity=walking&intensity=moderate&duration_minutes=15&weight_kg=70'
    rec='/api/v1/calories/recommendations?target_kcal=200&weight_kg=70&max_duration_minutes=45'
    call('forward calorie estimate',est,verify=lambda o:abs(o['data']['estimated_kcal']-69.825)<0.2)
    call('inverse calorie estimate',rec,verify=lambda o:bool(o['data']['recommendations']) and all(r['recommended_duration_minutes']<=45 and r['fits_constraints'] for r in o['data']['recommendations']))
    call('workout preview','/api/v1/workouts/generate','POST',PLAN,verify=valid_plan)
    tomorrow=(datetime.date.today()+datetime.timedelta(days=1)).isoformat()
    program={'goal':'endurance','workout_type':'cardio','experience_level':'beginner','weeks':2,'days_per_week':3,'start_date':tomorrow,'timezone':'Asia/Jakarta','available_days':['monday','wednesday','saturday'],'session_duration_minutes':30,'environment':'outdoor','space':'unrestricted','seed':42,'save':False}
    call('program preview','/api/v1/programs/generate','POST',program,verify=has_data)
    call('exercise alternatives','/api/v1/exercises/ex_barbell_bench_press/alternatives','POST',{'available_equipment':['bodyweight'],'experience_level':'beginner','environment':'home','space':'small','goal':'strength','max_results':5},verify=has_data)
    call('private route rejects anonymous','/api/v1/me',expected=401)
    call('invalid API key rejected','/api/v1/gyms',headers={'X-API-Key':'invalid-test-key'},expected=401)
    if args.partner:
        key=getpass.getpass('Full-scope test partner API key: ')
        h={'X-API-Key':key}
        call('partner catalog','/api/v1/equipment',headers=h,verify=has_data)
        call('partner gym data','/api/v1/gyms',headers=h,verify=has_data)
        call('partner calories',est,headers=h,verify=has_data)
        call('partner planner','/api/v1/workouts/generate','POST',PLAN,headers=h,verify=valid_plan)
        call('partner cannot read user data','/api/v1/me',headers=h,expected=401)
    if args.demo_email:
        password=getpass.getpass('Synthetic demo password: ')
        creds={'email':args.demo_email,'password':password}
        if args.register_demo:call('register demo','/api/v1/auth/register','POST',creds,expected=201,verify=has_data)
        login,_=call('demo login','/api/v1/auth/login','POST',creds,verify=lambda o:bool(o['data']['access_token']))
        if login:
            tokens=login['data']; h={'Authorization':'Bearer '+tokens['access_token']}
            call('demo user','/api/v1/me',headers=h,verify=lambda o:o['data']['email']==args.demo_email)
            call('demo workouts','/api/v1/workouts',headers=h,verify=has_data)
            call('demo logs','/api/v1/workout-logs',headers=h,verify=has_data)
            if args.write_demo_workout:
                saved,_=call('save synthetic workout','/api/v1/workouts/generate','POST',dict(PLAN,save=True),headers=h,verify=lambda o:o['data']['saved'] is True and bool(o['data']['id']))
                if saved:
                    w=saved['data']; wid=w['id']; item=items(saved)[0]
                    call('read saved workout','/api/v1/workouts/'+wid,headers=h,verify=has_data)
                    now=datetime.datetime.now(datetime.timezone.utc)
                    body={'status':'completed','started_at':(now-datetime.timedelta(minutes=20)).isoformat(timespec='seconds'),'finished_at':now.isoformat(timespec='seconds'),'items':[{'item_id':item['item_id'],'exercise_id':item['exercise_id'],'actual_duration_seconds':60}]}
                    call('record synthetic activity','/api/v1/workouts/'+wid+'/complete','POST',body,headers=dict(h,**{'Idempotency-Key':str(uuid.uuid4())}),expected=201,verify=has_data)
                    call('activity history','/api/v1/workout-logs',headers=h,verify=has_data)
                    call('activity summary','/api/v1/stats/summary',headers=h,verify=has_data)
            refreshed,_=call('refresh demo session','/api/v1/auth/refresh','POST',{'refresh_token':tokens['refresh_token']},verify=lambda o:bool(o['data']['access_token']))
            if refreshed:h={'Authorization':'Bearer '+refreshed['data']['access_token']}
            call('logout demo','/api/v1/auth/logout','POST',headers=h,expected=204)
    if args.benchmark and not fail:
        print('\nSequential latency sample (12 requests/route; no saturation test):')
        for name,path,method,payload in [('gym list','/api/v1/gyms','GET',None),('workout preview','/api/v1/workouts/generate','POST',PLAN),('calorie recommendations',rec,'GET',None)]:
            samples=[]
            for i in range(12):
                obj,ms=call('benchmark '+name,path,method,payload,quiet=True)
                if ms is None:break
                samples.append(ms);time.sleep(.2)
            if samples:
                ordered=sorted(samples); p95=ordered[math.ceil(.95*len(ordered))-1]
                print('{}: n={} median={:.1f} ms p95={:.1f} ms max={:.1f} ms'.format(name,len(samples),statistics.median(samples),p95,max(samples)))
    print('\n'+('FAILED: '+', '.join(fail) if fail else 'All requested checks passed.'))
    return bool(fail)
if __name__=='__main__':sys.exit(main())
