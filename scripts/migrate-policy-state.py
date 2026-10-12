#!/usr/bin/env python3
"""Run with CPA stopped. Move legacy static policy from JSON into YAML."""
import argparse,datetime,json,os,shutil
from pathlib import Path
import yaml
p=argparse.ArgumentParser();p.add_argument('config');a=p.parse_args();path=Path(a.config);c=yaml.safe_load(path.read_text());q=c['plugins']['configs']['cpa-key-policy'];state=Path(q['state_file']);s=json.loads(state.read_text())
if not any(s.get(k) for k in ('keys','aliases','classify_rules')):
 print('State already contains only runtime data');raise SystemExit
stamp=datetime.datetime.now().strftime('%Y%m%dT%H%M%S')
for f in [path,state]:shutil.copy2(f,str(f)+'.pre-static-policy-'+stamp)
seeds={k['id']:k for k in q.get('keys',[])};keys=[]
for k in s.get('keys',[]):
 k={x:v for x,v in k.items() if x not in ('created_at','updated_at','models','key_preview') and v is not None}
 seed=seeds.get(k['id'],{})
 if seed.get('key'):k['key']=seed['key'];k.pop('key_hash',None)
 keys.append(k)
q['keys']=keys
for field,identity in [('aliases','alias'),('classify_rules','name')]:
 items={x[identity]:x for x in s.get(field,[])};items.update({x[identity]:x for x in q.get(field,[])})
 if items:q[field]=list(items.values())
if 'global_weighted_round_robin' in s:q['global_weighted_round_robin']=s['global_weighted_round_robin']
path.write_text(yaml.safe_dump(c,sort_keys=False,allow_unicode=True));os.chmod(path,0o600)
state.write_text(json.dumps({'version':2,'usage':s.get('usage',{}),'updated_at':s.get('updated_at')},indent=2)+'\n');os.chmod(state,0o600)
print('Moved static policy to YAML; kept usage in JSON')
