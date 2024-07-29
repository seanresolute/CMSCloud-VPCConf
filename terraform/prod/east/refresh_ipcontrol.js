const AWS = require('aws-sdk')
const https = require('https');

// Later keys take priority
const validKeys = [ 'ipcontrol-refresh' ];

AWS.config.update({
    region: 'us-east-1'
  })
const parameterStore = new AWS.SSM()
const getParam = param => {
  return new Promise((res, rej) => {
    parameterStore.getParameter({
      Name: param,
      WithDecryption: true
    }, (err, data) => {
        if (err) {
          return rej(err)
        }
        return res(data)
    })
  })
}
const postRequest = options => {
  return new Promise((resolve, reject) => {
    const req = https.request(options, res => {
      let rawData = '';

      res.on('data', chunk => {
        rawData += chunk;
      });

      res.on('end', () => {
        try {
          resolve(JSON.parse(rawData));
        } catch (err) {
          console.log(rawData);
          reject(new Error(err));
        }
      });
    });

    req.on('error', err => {
      reject(new Error(err));
    });

    req.end();
  });
}

exports.handler = async function(event) {
  const param = await getParam('vpc-conf-prod-api-key-config');
  const keys = JSON.parse(param.Parameter.Value);
  let key = '';
  for (var principal in keys) {
    if (validKeys.includes(keys[principal].principal)) {
      console.log('Found key for ' + keys[principal].principal);
      key = keys[principal].keys[0];
    }
  }
  if (key == '') {
    console.log('No usable key found, bailing');
    return;
  }
  console.log('Found a key, proceed');
  console.log(await postRequest({
    hostname: 'vpc-conf.actually-east.west.cms.gov',
    path: '/provision/ipusage/refresh',
    method: 'POST',
    port: 443,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + key,
    },
  }));
}