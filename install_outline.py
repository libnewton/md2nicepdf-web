import requests,json,os


def install_outline():

    s = requests.Session()
    s.get('http://mdpdf:3000/')
    headers = {
        'Content-Type': 'application/x-www-form-urlencoded',
        "x-csrf-token": s.cookies['csrfToken']
    }
    data = {
        'teamName': 'pdf',
        'userName': 'pdf',
        'userEmail': 'pdf@example.com',
    }
    response = s.post('http://mdpdf:3000/api/installation.create',  headers=headers, data=data)
    tokenResp = s.post('http://mdpdf:3000/api/auth.info', headers={"Content-Type": "application/json"})
    collCreateData = {"name":"pdf","icon":"collection","sharing":True,"commenting":True,"color":"#00D084"}
    csrfToken = s.cookies['csrfToken']
    rColl = s.post('http://mdpdf:3000/api/collections.create', json=collCreateData, headers={"Content-Type": "application/json", "X-Csrf-Token": csrfToken, "Cookie": f"csrfToken={csrfToken}; accessToken={s.cookies['accessToken']}"})
    collectionId = json.loads(rColl.text)['data']['id']
    apiKeyR = s.post('http://mdpdf:3000/api/apiKeys.create', json={"name":"pdf"}, headers={"Content-Type": "application/json", "X-Csrf-Token": csrfToken, "Cookie": f"csrfToken={csrfToken}; accessToken={s.cookies['accessToken']}"})
    apiKey = json.loads(apiKeyR.text)['data']['value']
    return {'collectionId': collectionId, "apiKey": apiKey}
if os.path.isfile("/outline.json"):
    print("Outline already installed.")
    exit(0)

installed = install_outline()
with open("/outline.json", "w") as f:
    json.dump(installed, f)