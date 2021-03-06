/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
package org.apache.beam.sdk.io.gcp.pubsublite;

import com.google.cloud.pubsublite.Offset;
import com.google.cloud.pubsublite.internal.CheckedApiException;
import com.google.cloud.pubsublite.internal.PullSubscriber;
import com.google.cloud.pubsublite.proto.SequencedMessage;
import java.util.List;
import java.util.Optional;
import java.util.stream.Collectors;

/**
 * A PullSubscriber translating from {@link com.google.cloud.pubsublite.SequencedMessage}to {@link
 * com.google.cloud.pubsublite.proto.SequencedMessage}.
 */
class TranslatingPullSubscriber implements PullSubscriber<SequencedMessage> {
  private final PullSubscriber<com.google.cloud.pubsublite.SequencedMessage> underlying;

  TranslatingPullSubscriber(
      PullSubscriber<com.google.cloud.pubsublite.SequencedMessage> underlying) {
    this.underlying = underlying;
  }

  @Override
  public List<SequencedMessage> pull() throws CheckedApiException {
    List<com.google.cloud.pubsublite.SequencedMessage> messages = underlying.pull();
    return messages.stream().map(m -> m.toProto()).collect(Collectors.toList());
  }

  @Override
  public Optional<Offset> nextOffset() {
    return underlying.nextOffset();
  }

  @Override
  public void close() throws Exception {
    underlying.close();
  }
}
