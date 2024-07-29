const AWS = require('aws-sdk')
const https = require('https');

// Later keys take priority
const validKeys = [ 'ipcontrol-refresh' ];

AWS.config.update({
    region: 'us-east-1'
  })
const parameterStore = new AWS.SSM();
const cloudwatchService = new AWS.CloudWatch();
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
const putMetrics = metrics => {
  const metricData = [];
  for (var index in metrics) {
    metricData.push({
      MetricName: 'Percent Free',
      Dimensions: [
        {
            Name: 'Region',
            Value: metrics[index].Region,
        },
        {
            Name: 'Zone',
            Value: metrics[index].Zone,
        }
      ],
      Unit: 'Percent',
      Value: metrics[index].IPFreePercent * 100
    });
    metricData.push({
      MetricName: 'Free IP',
      Dimensions: [
        {
            Name: 'Region',
            Value: metrics[index].Region,
        },
        {
            Name: 'Zone',
            Value: metrics[index].Zone,
        }
      ],
      Unit: 'None',
      Value: metrics[index].IPFree
    });
  }
  return new Promise((resolve, reject) => {
    cloudwatchService.putMetricData({
        MetricData: metricData,
        Namespace: 'ipcontrol-utilization'
    }, (err, data) => {
      if (err) {
        return reject(err)
      }
      return resolve(data);
    })
  })
}

const sendRequest = options => {
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
  const param = await getParam('vpc-conf-dev-api-key-config');
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
  let result = await sendRequest({
    hostname: 'vpc-conf.actually-east.west.cms.gov',
    path: '/provision/ipusage.json',
    method: 'GET',
    port: 443,
    headers: {
      'Content-Type': 'application/json',
      'Authorization': 'Bearer ' + key,
    },
  });
  for (var index in result.Data) {
    let entry = result.Data[index];
    console.log("{" + entry.Region + "," + entry.Zone + "}: " + entry.IPFree + " (" + entry.IPFreePercent * 100 + ")");
  }
  let response = await putMetrics(result.Data);
  console.log(response);
//   console.log(result);
}
