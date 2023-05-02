import requests
import json
from typing import List

# Define the URL for the Grafana API
grafana_url = ""

# Set the headers for the API request
headers = {
    'Content-Type': 'application/json',
    'Authorization': 'Bearer XXX'
}

def remove_dashboard(dashboard_uid):
    url = grafana_url + '/dashboards/uid/' + dashboard_uid
    # Send the delete request
    response = requests.delete(url, headers=headers)

    # Check if the request was successful
    if response.status_code == 200:
        print('Dashboard deleted successfully')
    else:
        print('Failed to delete dashboard: {}'.format(response.text))

def create_dashboard(dashboard_payload):
    # Send a POST request to create the new dashboard
    response = requests.post(grafana_url + '/dashboards/db', headers=headers, json=dashboard_payload)

    # Verify that the dashboard was created successfully
    if response.status_code == 200:
        print('Dashboard created successfully')
    else:
        print('Failed to create dashboard: ' + response.text)


# Define the JSON payload for the new dashboard
def get_row(title):
    row = {
      "datasource": 'default',
      "gridPos": {
        "h": 1,
        "w": 24,
        "x": 0,
        "y": 0
      },
      "id": None,
      "panels": [],
      "targets": [
        {
          "datasource": 'default',
          "refId": "A"
        }
      ],
      "title": title,
      "type": "row"
    }

    return row

def get_grafana_dashboard_payload_httpapi(form_payload, dashboard_uid:str, dashboard_title: str=None, dashboard_tags: List[str]=["Opni-metricAI"]):
    """
    Generate the json format payload for grafana dashboard http api
    params:
    @form_payload: List of tuples, each item contains information of a panel in the dashboard - (type_pattern, panel_title, query):
                            type_pattern: the pattern predicted from CNN model
                            panel_title: title of the panel, contains information of the metric name and pod name
                            query: the promQL query for this panel
    @dashboard_uid: the unique id of the dashboard, which can be used to identify/create/update/delete the dashboard.
    @dashboard_title: a string that is simple the title of the dashboard
    @dashboard_tags: List[str], the tags of the dashboard
    """
    if dashboard_title is None:
        dashboard_title = "Opni-metricAI-" +  dashboard_uid
    panels1, panels2 = [get_row("Type1")], [get_row("Type2")]
    for type_pattern, panel_title ,query in form_payload:
        ## definition of each panel
        p= {
                'id': None,
                'type': 'graph',
                'title': panel_title,
                'datasource': 'default',
                'targets': [{
                    'refId': 'A',
                    "expr": query
                }],
                'xaxis': {
                    'show': True
                },
                'yaxes': [{
                    'format': 'short',
                    'label': 'Count',
                    'logBase': 1,
                    'max': None,
                    'min': 0,
                    'show': True
                }, {
                    'format': 'short',
                    'label': None,
                    'logBase': 1,
                    'max': None,
                    'min': None,
                    'show': False
                }],
                'legend': {
                    'alignAsTable': True,
                    'avg': True,
                    'current': True,
                    'hideEmpty': False,
                    'hideZero': False,
                    'max': True,
                    'min': True,
                    'rightSide': True,
                    'show': True,
                    'sortDesc': True,
                    'sort': 'total',
                    'total': True,
                    'values': True
                },
                'gridPos': {
                    'h': 8,
                    'w': 12,
                    'x': 0,
                    'y': 0
                },
                'tooltip': {
                    'shared': True,
                    'sort': 0,
                    'value_type': 'individual'
                },
                'links': [],
                'maxDataPoints': 100,
                'nullPointMode': 'null',
                'pointradius': 5,
                'stack': False,
                'steppedLine': False,
                'timeFrom': None,
                'timeShift': None,
                'options': {
                    'showThresholdLabels': False,
                    'showThresholdMarkers': True
                },
                'pluginVersion': '7.4.3',
                'thresholds': []
            }
        if "type1" in type_pattern:
            panels1.append(p)
        else:
            panels2.append(p)
    panels1.extend(panels2)
    ## dashboard metadata
    dashboard_payload = {
        'dashboard': {
            'id': None,
            'uid': dashboard_uid,
            'title': dashboard_title,
            'tags': dashboard_tags,
            'timezone': 'browser',
            'schemaVersion': 22,
            'version': 0,
            'refresh': '30s',
            'time': {
                'from': 'now-1h',
                'to': 'now'
            },
            'panels': panels1
        },
        'folderId': 0,
        'overwrite': False
    }
    return dashboard_payload


def get_grafana_dashboard_payload(form_payload, dashboard_uid:str, dashboard_title: str="", dashboard_tags: List[str]=["Opni-metricAI"]):
    """
    Generate the json format payload for grafana dashboard k8s 
    params:
    @form_payload: List of tuples, each item contains information of a panel in the dashboard - (type_pattern, panel_title, query):
                            type_pattern: the pattern predicted from CNN model
                            panel_title: title of the panel, contains information of the metric name and pod name
                            query: the promQL query for this panel
    @dashboard_uid: the unique id of the dashboard, which can be used to identify/create/update/delete the dashboard.
    @dashboard_title: a string that is simple the title of the dashboard
    @dashboard_tags: List[str], the tags of the dashboard
    """
    if dashboard_title == "":
        dashboard_title = "Opni-metricAI-" +  dashboard_uid
    panels1, panels2 = [get_row("Type1")], [get_row("Type2")]
    for type_pattern, panel_title ,query in form_payload:
        ## definition of each panel
        p= {
            "datasource": {
                "type": "default"
            },
            "fieldConfig": {
                "defaults": {
                "color": {
                    "mode": "palette-classic"
                },
                "custom": {
                    "axisCenteredZero": False,
                    "axisColorMode": "text",
                    "axisLabel": "",
                    "axisPlacement": "auto",
                    "barAlignment": 0,
                    "drawStyle": "line",
                    "fillOpacity": 0,
                    "gradientMode": "none",
                    "hideFrom": {
                    "legend": False,
                    "tooltip": False,
                    "viz": False
                    },
                    "lineInterpolation": "linear",
                    "lineWidth": 1,
                    "pointSize": 5,
                    "scaleDistribution": {
                    "type": "linear"
                    },
                    "showPoints": "auto",
                    "spanNulls": False,
                    "stacking": {
                    "group": "A",
                    "mode": "none"
                    },
                    "thresholdsStyle": {
                    "mode": "off"
                    }
                },
                "mappings": [],
                "thresholds": {
                    "mode": "absolute",
                    "steps": [
                    {
                        "color": "green",
                        "value": None
                    },
                    {
                        "color": "red",
                        "value": 80
                    }
                    ]
                }
                },
                "overrides": []
            },
            "gridPos": {
                "h": 8,
                "w": 12,
                "x": 0,
                "y": 1
            },
            "id": 4,
            "options": {
                "legend": {
                "calcs": [],
                "displayMode": "list",
                "placement": "bottom",
                "showLegend": True
                },
                "tooltip": {
                "mode": "single",
                "sort": "none"
                }
            },
            "targets": [
                {
                "datasource": {
                    "type": "default"
                },
                "editorMode": "builder",
                "expr": query,
                "legendFormat": "__auto",
                "range": True,
                "refId": "A"
                }
            ],
            "title": panel_title,
            "type": "timeseries"
            }
        if "type1" in type_pattern:
            panels1.append(p)
        else:
            panels2.append(p)
    panels1.extend(panels2)
    ## dashboard metadata
    dashboard_payload = {
        "fiscalYearStartMonth": 0,
        "graphTooltip": 0,
        "id": 0,
        "links": [],
        "panels": panels1,
        "refresh": "30s",
        "revision": 1,
        "schemaVersion": 38,
        "tags": dashboard_tags,
        "templating": {
            "list": []
        },
        "time": {
            "from": "now-1h",
            "to": "now"
        },
        "timepicker": {},
        "timezone": "browser",
        "title": dashboard_title,
        "uid": dashboard_uid,
        "version": 1,
        "weekStart": ""
        }
    return dashboard_payload