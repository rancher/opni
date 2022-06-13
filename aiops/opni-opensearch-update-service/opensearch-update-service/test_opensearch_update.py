import asyncio
import pandas as pd
import unittest
from unittest import IsolatedAsyncioTestCase

import sys
from main import doc_generator

class TestStringMethods(IsolatedAsyncioTestCase):
    #Testing one entry being sent over to the doc_generator function.
    async def test_doc_generator_one_doc(self):
        entry_processed = pd.DataFrame([{"log": "Initialization completed", "masked_log": "initialization completed", "anomaly_level": "Normal"}])
        processed_results = doc_generator(entry_processed)
        async for res in processed_results:
            self.assertEqual(res["_op_type"], "update")
            self.assertEqual(res["_index"], "logs")
            self.assertEqual(res["doc"]["entry_processed"], True)
            self.assertEqual(res["doc"]["log"], "Initialization completed")
            self.assertEqual(res["doc"]["masked_log"], "initialization completed")
            self.assertEqual(res["doc"]["anomaly_level"], "Normal")


    async def test_doc_generator_multiple_doc(self):
        # Testing multiple entries being sent over to the doc_generator function.
        all_entries = [{"log": "Successfully connected to Redis", "masked_log": "successfully connected to redis", "anomaly_level": "Normal"}, {"log": "Application is shutting down...", "masked_log": "application is shutting down...", "anomaly_level": "Normal"}, {"log": "Now listening on: http://[::]:7070", "masked_log": "now listening on : http : // : : : <num>", "anomaly_level": "Normal"}, {"log": "Application started. Press Ctrl+C to shut down.", "masked_log": "application started. press ctrl+c to shut down.", "anomaly_level": "Normal"}, {"log": "Content root path: /app", "masked_log": "content root path : <path>", "anomaly_level": "Normal"}, {"log": "Hosting environment: Production", "masked_log": "hosting environment : production", "anomaly_level": "Normal"}, {"log": "Initialization completed", "masked_log": "initialization completed", "anomaly_level": "Normal"}, {"log": "Small test result: OK", "masked_log": "small test result : ok", "anomaly_level": "Normal"}, {"log": "Performing small test", "masked_log": "performing small test", "anomaly_level": "Normal"}, {"log": "Successfully connected to Redis", "masked_log": "successfully connected to redis", "anomaly_level": "Normal"}, {"log": "Application is shutting down...", "masked_log": "application is shutting down...", "anomaly_level": "Normal"}, {"log": "@google-cloud/profiler Failed to create profile, waiting 3.5s to try again: Error: Could not load the default credentials. Browse to https://cloud.google.com/docs/authentication/getting-started for more information.", "masked_log": "@google-cloud<path> failed to create profile waiting <duration> to try again : error : could not load the default credentials. browse to <url> for more information.", "anomaly_level": "Anomaly"}, {"log": "@google-cloud/profiler Failed to create profile, waiting 5.5s to try again: Error: Could not load the default credentials. Browse to https://cloud.google.com/docs/authentication/getting-started for more information.", "masked_log": "@google-cloud<path> failed to create profile waiting <duration> to try again : error : could not load the default credentials. browse to <url> for more information.", "anomaly_level": "Anomaly"}]
        all_processed_entries = pd.DataFrame(all_entries)
        processed_results = doc_generator(all_processed_entries)
        idx = 0
        async for res in processed_results:
            self.assertEqual(res["_op_type"], "update")
            self.assertEqual(res["_index"], "logs")
            self.assertEqual(res["doc"]["entry_processed"], True)
            self.assertEqual(res["doc"]["log"], all_entries[idx]["log"])
            self.assertEqual(res["doc"]["masked_log"], all_entries[idx]["masked_log"])
            self.assertEqual(res["doc"]["anomaly_level"], all_entries[idx]["anomaly_level"])
            idx += 1